package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/config"
	"github.com/mr-exz/cibot/internal/linear"
	"github.com/mr-exz/cibot/internal/storage"
)

type stateKey struct {
	UserID int64
}

type Handler struct {
	linear      *linear.Client
	storage     *storage.DB
	cfg         *config.Config
	version     string
	mu          sync.Mutex
	states      map[stateKey]interface{} // can hold *pendingSession or *pendingAdminSession
	topics      map[int64]map[int]string // chat_id -> (thread_id -> topic_name)
	groups      map[int64]string         // chat_id -> group title (discovered from messages)
	cmdRegistry []commandDef
	cmdHandlers map[string]cmdHandler
}

func New(ctx context.Context, linearClient *linear.Client, db *storage.DB, cfg *config.Config, version string) (*tgbot.Bot, error) {
	h := &Handler{
		linear:      linearClient,
		storage:     db,
		cfg:         cfg,
		version:     version,
		states:      make(map[stateKey]interface{}),
		topics:      make(map[int64]map[int]string),
		groups:      make(map[int64]string),
		cmdHandlers: make(map[string]cmdHandler),
	}

	// Build command registry — single source of truth for dispatch and /help
	for _, cmd := range h.registerCommands() {
		h.cmdRegistry = append(h.cmdRegistry, cmd)
		h.cmdHandlers[cmd.Name] = cmd.Handler
	}
	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(h.handleMessage),
	}
	b, err := tgbot.New(cfg.TelegramToken, opts...)
	if err != nil {
		return nil, err
	}

	// Register callback query handlers
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "cat:", tgbot.MatchTypePrefix, h.handleCategoryCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "type:", tgbot.MatchTypePrefix, h.handleRequestTypeCallback)

	// Admin flow callbacks
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "grp:", tgbot.MatchTypePrefix, h.handleAdminTopicGroupCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "person:", tgbot.MatchTypePrefix, h.handleAdminPersonCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "rot:", tgbot.MatchTypePrefix, h.handleAdminRotationCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "skip", tgbot.MatchTypeExact, h.handleAdminSkipCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "confirm:", tgbot.MatchTypePrefix, h.handleAdminConfirmCallback)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "topic:", tgbot.MatchTypePrefix, h.handleAdminTopicManualCallback)

	go h.sessionReaper(ctx)

	return b, nil
}

// sessionReaper runs in the background and removes sessions that have been
// inactive for more than sessionTTL, freeing memory from abandoned flows.
const sessionTTL = 30 * time.Minute

func (h *Handler) sessionReaper(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sweepSessions()
		}
	}
}

func (h *Handler) sweepSessions() {
	cutoff := time.Now().Add(-sessionTTL)
	h.mu.Lock()
	defer h.mu.Unlock()

	removed := 0
	for key, state := range h.states {
		var createdAt time.Time
		switch s := state.(type) {
		case *pendingSession:
			createdAt = s.CreatedAt
		case *pendingAdminSession:
			createdAt = s.CreatedAt
		}
		if !createdAt.IsZero() && createdAt.Before(cutoff) {
			delete(h.states, key)
			removed++
		}
	}
	if removed > 0 {
		log.Printf("🧹 Session reaper: removed %d expired sessions", removed)
	}
}

func (h *Handler) handleMessage(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	threadInfo := ""
	if msg.MessageThreadID != 0 {
		threadInfo = fmt.Sprintf(" [TOPIC #%d]", msg.MessageThreadID)
	}
	log.Printf("📨 Message from chat_id: %d, user: %s (@%s), text: %s%s\n", msg.Chat.ID, msg.From.FirstName, msg.From.Username, msg.Text, threadInfo)

	// Cache group name for use in /addtopic flow
	if msg.Chat.Title != "" {
		h.mu.Lock()
		h.groups[msg.Chat.ID] = msg.Chat.Title
		h.mu.Unlock()
	}

	// Only process messages from configured chats or private chats from admins
	chatType := string(msg.Chat.Type)
	if !h.cfg.IsPrivateChatAllowed(msg.Chat.ID, chatType, msg.From.Username) {
		reason := "not in ALLOWED_CHAT_ID"
		if chatType == "private" {
			reason = "not an admin (DMs only for admins)"
		}
		log.Printf("⏭️ Ignoring message from chat_id: %d (%s)\n", msg.Chat.ID, reason)
		return
	}

	key := stateKey{
		UserID: msg.From.ID,
	}

	// Check if user is in pending state (waiting for description)
	h.mu.Lock()
	pending, hasPending := h.states[key]
	h.mu.Unlock()

	if hasPending && !isCommand(msg.Text) {
		// Handle pending sessions (support/ticket flows)
		if pendingSess, ok := pending.(*pendingSession); ok {
			if pendingSess.Flow == FlowSupport {
				h.handleSupportPendingIssue(ctx, b, msg, pendingSess)
			}
			return
		}

		// Handle admin pending sessions
		if adminPending, ok := pending.(*pendingAdminSession); ok {
			h.handleAdminPendingInput(ctx, b, msg, adminPending)
			return
		}
	}

	// Parse command (handles both /help and /help@botname)
	cmd := parseCommand(msg.Text)
	if cmd == "" {
		return
	}

	if cmd == "help" {
		params := &tgbot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   h.buildHelpText(msg.From.Username),
		}
		if msg.MessageThreadID != 0 {
			params.MessageThreadID = msg.MessageThreadID
		}
		if _, err := b.SendMessage(ctx, params); err != nil {
			log.Printf("❌ Failed to send help: %v", err)
		}
		return
	}

	if handler, ok := h.cmdHandlers[cmd]; ok {
		// Enforce admin-only restriction at dispatch level
		for _, def := range h.cmdRegistry {
			if def.Name == cmd && def.AdminOnly && !isAdmin(h.cfg, msg.From.Username) {
				h.sendMessage(ctx, b, msg, "Access denied.")
				return
			}
		}
		handler(ctx, b, msg)
	}
}

func (h *Handler) sendMessage(ctx context.Context, b *tgbot.Bot, msg *models.Message, text string) {
	params := &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   text,
	}
	// If message is in a topic, reply in the same topic
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}
	b.SendMessage(ctx, params)
}

// parseCommand extracts command from text, handling both /help and /help@botname formats
func parseCommand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}

	// Split by space to get the first word
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}

	cmd := parts[0]
	// Remove leading /
	cmd = strings.TrimPrefix(cmd, "/")
	// Remove bot mention (@botname)
	if idx := strings.Index(cmd, "@"); idx != -1 {
		cmd = cmd[:idx]
	}

	return cmd
}

// isCommand checks if text starts with a slash command
func isCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

// recordTopic stores a discovered topic in memory and database
func (h *Handler) recordTopic(chatID int64, threadID int, topicName string) {
	h.mu.Lock()
	if h.topics[chatID] == nil {
		h.topics[chatID] = make(map[int]string)
	}
	h.topics[chatID][threadID] = topicName
	h.mu.Unlock()

	// Also save to database (best effort)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.storage.SaveTopic(ctx, chatID, threadID, topicName); err != nil {
		log.Printf("⚠️  Failed to save topic to DB: %v", err)
	}
}

// getGroupName returns the cached title for a group, or a fallback string
func (h *Handler) getGroupName(chatID int64) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if name, ok := h.groups[chatID]; ok {
		return name
	}
	return fmt.Sprintf("Group %d", chatID)
}

// getAllTopics returns topics for all known groups (cache + config IDs)
func (h *Handler) getAllTopics() map[int64]map[int]string {
	h.mu.Lock()
	knownIDs := make([]int64, 0, len(h.groups))
	for id := range h.groups {
		knownIDs = append(knownIDs, id)
	}
	h.mu.Unlock()

	// Merge config chat IDs
	for _, id := range h.cfg.AllowedChatIDs {
		found := false
		for _, kid := range knownIDs {
			if kid == id {
				found = true
				break
			}
		}
		if !found {
			knownIDs = append(knownIDs, id)
		}
	}

	result := make(map[int64]map[int]string)
	for _, chatID := range knownIDs {
		topics := h.getTopics(chatID)
		if len(topics) > 0 {
			result[chatID] = topics
		}
	}
	return result
}

// getKnownGroups returns all known groups from cache + configured allowed chat IDs
func (h *Handler) getKnownGroups() map[int64]string {
	h.mu.Lock()
	result := make(map[int64]string, len(h.groups))
	for k, v := range h.groups {
		result[k] = v
	}
	h.mu.Unlock()

	// Fill in any allowed chat IDs not yet seen in cache
	for _, chatID := range h.cfg.AllowedChatIDs {
		if _, ok := result[chatID]; !ok {
			result[chatID] = fmt.Sprintf("Group %d", chatID)
		}
	}
	return result
}

// getTopics returns all discovered topics for a chat (loads from DB if not in memory)
func (h *Handler) getTopics(chatID int64) map[int]string {
	h.mu.Lock()
	cached := h.topics[chatID]
	h.mu.Unlock()

	// If not in memory, try loading from database
	if cached == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		topics, err := h.storage.LoadTopicsForChat(ctx, chatID)
		if err != nil {
			log.Printf("⚠️  Failed to load topics from DB: %v", err)
			return make(map[int]string)
		}

		// Store in memory for future use
		h.mu.Lock()
		h.topics[chatID] = topics
		h.mu.Unlock()

		return topics
	}

	// Return a copy to avoid race conditions
	topicsCopy := make(map[int]string)
	for k, v := range cached {
		topicsCopy[k] = v
	}
	return topicsCopy
}
