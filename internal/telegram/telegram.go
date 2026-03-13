package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/linear"
)

type Handler struct {
	linear *linear.Client
}

func New(ctx context.Context, linearClient *linear.Client) (*tgbot.Bot, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	h := &Handler{linear: linearClient}
	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(h.handleMessage),
	}
	return tgbot.New(token, opts...)
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
		h.handleIssueCommand(ctx, b, msg)
	}
}

func (h *Handler) handleIssueCommand(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Extract title and description from message
	// Format: /issue title | description (description optional)
	text := msg.Text

	// Find where the command ends (handle /issue@botname)
	cmdEnd := strings.IndexAny(text, " ")
	if cmdEnd == -1 {
		h.sendMessage(ctx, b, msg, "❌ Usage: /issue <title> | <description (optional)>")
		return
	}

	// Everything after the command
	content := strings.TrimSpace(text[cmdEnd:])
	if content == "" {
		h.sendMessage(ctx, b, msg, "❌ Usage: /issue <title> | <description (optional)>")
		return
	}

	var title, description string
	if idx := strings.Index(content, "|"); idx != -1 {
		title = strings.TrimSpace(content[:idx])
		description = strings.TrimSpace(content[idx+1:])
	} else {
		title = content
	}

	if title == "" {
		h.sendMessage(ctx, b, msg, "❌ Title cannot be empty")
		return
	}

	// Create the issue
	url, err := h.linear.CreateIssue(ctx, title, description)
	if err != nil {
		log.Printf("❌ Failed to create Linear issue: %v\n", err)
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to create issue: %v", err))
		return
	}

	h.sendMessage(ctx, b, msg, fmt.Sprintf("✓ Issue created: %s", url))
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
