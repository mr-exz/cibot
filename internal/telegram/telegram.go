package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/linear"
)

type Category struct {
	ID    string
	Label string
}

var categories = []Category{
	{ID: "network", Label: "🌐 Network"},
	{ID: "vm_k8s", Label: "☁️ VM/K8s"},
	{ID: "database", Label: "🗄️ Database"},
	{ID: "cicd", Label: "⚙️ CI/CD"},
	{ID: "incident", Label: "🚨 Incident"},
	{ID: "feature_request", Label: "✨ Feature Request"},
	{ID: "access_request", Label: "🔑 Access Request"},
}

type stateKey struct {
	UserID int64
}

type pendingIssue struct {
	Category    string
	Title       string
	Step        string // "category", "title", "description"
	MessageID   int    // Message ID to edit throughout the flow
	ChatID      int64  // Chat ID for editing the message
}

type Handler struct {
	linear *linear.Client
	mu     sync.Mutex
	states map[stateKey]*pendingIssue
}

func New(ctx context.Context, linearClient *linear.Client) (*tgbot.Bot, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	h := &Handler{
		linear: linearClient,
		states: make(map[stateKey]*pendingIssue),
	}
	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(h.handleMessage),
	}
	b, err := tgbot.New(token, opts...)
	if err != nil {
		return nil, err
	}

	// Register callback query handler for category selection
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "cat:", tgbot.MatchTypePrefix, h.handleCategoryCallback)

	return b, nil
}

func (h *Handler) handleMessage(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	log.Printf("📨 Message from chat_id: %d, user: %s, text: %s\n", msg.Chat.ID, msg.From.FirstName, msg.Text)

	// Only process messages from the configured group chat
	if !isAllowedChat(msg.Chat.ID) {
		log.Printf("⏭️  Ignoring message from chat_id: %d (not in ALLOWED_CHAT_ID)\n", msg.Chat.ID)
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
		h.handlePendingIssue(ctx, b, msg, pending)
		return
	}

	// Parse command (handles both /help and /help@botname)
	cmd := parseCommand(msg.Text)
	if cmd == "" {
		return
	}

	switch cmd {
	case "help":
		log.Printf("✓ Processing /help command from chat_id: %d\n", msg.Chat.ID)
		params := &tgbot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "hello world",
		}
		// If message is in a topic, reply in the same topic
		if msg.MessageThreadID != 0 {
			params.MessageThreadID = msg.MessageThreadID
		}
		b.SendMessage(ctx, params)

	case "issue":
		log.Printf("✓ Processing /issue command from chat_id: %d\n", msg.Chat.ID)
		h.handleIssueStart(ctx, b, msg)
	}
}

func (h *Handler) handleIssueStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Create inline keyboard with category buttons (2 per row)
	rows := make([][]models.InlineKeyboardButton, 0)
	for i := 0; i < len(categories); i += 2 {
		row := []models.InlineKeyboardButton{
			{
				Text:         categories[i].Label,
				CallbackData: fmt.Sprintf("cat:%s", categories[i].ID),
			},
		}
		if i+1 < len(categories) {
			row = append(row, models.InlineKeyboardButton{
				Text:         categories[i+1].Label,
				CallbackData: fmt.Sprintf("cat:%s", categories[i+1].ID),
			})
		}
		rows = append(rows, row)
	}

	params := &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Select issue category:",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		},
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}
	sentMsg, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	// Store message ID for later editing
	key := stateKey{
		UserID: msg.From.ID,
	}
	h.mu.Lock()
	h.states[key] = &pendingIssue{
		Step:      "category",
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
	}
	h.mu.Unlock()
}

func (h *Handler) handleCategoryCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	// Access the Message field from MaybeInaccessibleMessage
	msg := query.Message.Message
	if msg == nil {
		return
	}

	categoryID := strings.TrimPrefix(query.Data, "cat:")

	// Find category label
	var categoryLabel string
	for _, cat := range categories {
		if cat.ID == categoryID {
			categoryLabel = cat.Label
			break
		}
	}
	if categoryLabel == "" {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "❌ Unknown category",
			ShowAlert:       true,
		})
		return
	}

	// Answer callback to remove loading spinner
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            fmt.Sprintf("✓ %s selected", categoryLabel),
	})

	// Update state with category and move to title step
	key := stateKey{
		UserID: query.From.ID,
	}
	h.mu.Lock()
	if pending, exists := h.states[key]; exists {
		pending.Category = categoryID
		pending.Step = "title"
		// Use the stored message ID for editing
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("✓ %s selected.\n\n📝 **Title:**", categoryLabel),
		})
	}
	h.mu.Unlock()
}

func (h *Handler) handlePendingIssue(ctx context.Context, b *tgbot.Bot, msg *models.Message, pending *pendingIssue) {
	key := stateKey{
		UserID: msg.From.ID,
	}

	text := strings.TrimSpace(msg.Text)

	switch pending.Step {
	case "title":
		if text == "" {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      "❌ Title cannot be empty",
			})
			return
		}
		// Delete user's message
		b.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
			ChatID:    pending.ChatID,
			MessageID: msg.ID,
		})

		// Store title and move to description step
		h.mu.Lock()
		pending.Title = text
		pending.Step = "description"
		h.mu.Unlock()

		// Edit the message to ask for description
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("✓ Title: %s\n\n📄 **Description (optional):**", text),
		})

	case "description":
		// Delete user's message
		b.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
			ChatID:    pending.ChatID,
			MessageID: msg.ID,
		})

		// Create the issue with title + description
		h.mu.Lock()
		title := pending.Title
		delete(h.states, key)
		h.mu.Unlock()

		description := text // Empty string is OK for description

		// Create the issue
		url, err := h.linear.CreateIssue(ctx, title, description)
		if err != nil {
			log.Printf("❌ Failed to create Linear issue: %v\n", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      fmt.Sprintf("❌ Failed to create issue: %v", err),
			})
			return
		}

		// Edit the message with final result
		descText := description
		if descText == "" {
			descText = "(none)"
		}
		finalText := fmt.Sprintf("✓ Issue created!\n\n📝 Title: %s\n📄 Description: %s\n\n🔗 %s", title, descText, url)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      finalText,
		})
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

// isAllowedChat checks if the message is from the configured group chats
func isAllowedChat(chatID int64) bool {
	allowedChatStr := os.Getenv("ALLOWED_CHAT_ID")
	if allowedChatStr == "" {
		// If not configured, allow all chats
		return true
	}

	// Parse comma-separated list of chat IDs
	chatIDs := strings.Split(allowedChatStr, ",")
	for _, idStr := range chatIDs {
		idStr = strings.TrimSpace(idStr)
		allowedChat, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		if chatID == allowedChat {
			return true
		}
	}

	return false
}

// isCommand checks if text starts with a slash command
func isCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}
