package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/storage"
)

// techThreadKey returns the map key used for the in-memory tech thread cache.
func techThreadKey(chatID int64, threadID int) string {
	return fmt.Sprintf("%d:%d", chatID, threadID)
}

// handleThread starts the /thread flow. Shows a category picker — the selected
// category's Linear team key is used to create the issue.
func (h *Handler) handleThread(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if h.cfg.TechGroupID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ TECH_GROUP_ID is not configured. Ask the administrator to set it.")
		return
	}
	if msg.ReplyToMessage == nil {
		h.sendMessage(ctx, b, msg, "⚠️ /thread requires a reply. Reply to a message to open a technical thread for it.")
		return
	}
	if msg.MessageThreadID != 0 && msg.ReplyToMessage.ID == msg.MessageThreadID {
		h.sendMessage(ctx, b, msg, "⚠️ /thread requires a reply to a user message, not the topic header.")
		return
	}

	replied := msg.ReplyToMessage
	body := replied.Text
	if body == "" {
		body = replied.Caption
	}

	reporterName := ""
	reporterUsername := ""
	if replied.From != nil {
		reporterName = strings.TrimSpace(replied.From.FirstName + " " + replied.From.LastName)
		reporterUsername = replied.From.Username
	}

	categories, err := h.storage.ListCategoriesForContext(ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}
	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ No categories configured for this context.")
		return
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "🗂️ Select category for this thread:",
		ReplyMarkup:     buildCategoryKeyboard(categories),
	})
	if err != nil {
		log.Printf("❌ /thread: failed to send message: %v", err)
		return
	}

	session := &pendingSession{
		Flow:             FlowThread,
		Step:             StepCategory,
		UserID:           msg.From.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		MessageID:        sentMsg.ID,
		CreatedAt:        time.Now(),
		TicketMsgLink:    formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, replied.ID),
		TicketMsgBody:    body,
		TicketMsgDate:    time.Unix(int64(replied.Date), 0),
		SourceMsgID:      replied.ID,
		ReporterName:     reporterName,
		ReporterUsername: reporterUsername,
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = session
	h.mu.Unlock()
}

// completeTechThread is called from handleCategoryCallback when FlowThread.
// It creates the Linear issue + Telegram topic + thread file and saves the DB record.
func (h *Handler) completeTechThread(ctx context.Context, b *tgbot.Bot, pending *pendingSession) {
	reporter := pending.ReporterName
	if pending.ReporterUsername != "" {
		reporter = fmt.Sprintf("[%s](https://t.me/%s)", pending.ReporterName, pending.ReporterUsername)
	}

	title := ticketTitle(pending.TicketMsgBody, pending.TicketMsgDate)
	if strings.TrimSpace(pending.TicketMsgBody) == "" {
		title = fmt.Sprintf("Thread from Telegram (%s)", pending.TicketMsgDate.Format("2006-01-02 15:04"))
	}

	description := fmt.Sprintf("**Reporter:** %s\n**Category:** %s\n**Source:** %s\n\n%s",
		reporter, pending.CategoryName, pending.TicketMsgLink, pending.TicketMsgBody)

	issue, err := h.linear.CreateIssue(ctx, title, description, pending.TeamKey, "", nil, 0)
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to create Linear issue: %v", err),
		})
		return
	}

	topicName := issue.Identifier
	short := title
	if len(short) > 50 {
		short = short[:47] + "..."
	}
	if short != "" {
		topicName = fmt.Sprintf("%s: %s", issue.Identifier, short)
	}

	topic, err := b.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{
		ChatID: h.cfg.TechGroupID,
		Name:   topicName,
	})
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to create Telegram topic: %v", err),
		})
		return
	}

	b.ForwardMessage(ctx, &tgbot.ForwardMessageParams{
		ChatID:          h.cfg.TechGroupID,
		MessageThreadID: topic.MessageThreadID,
		FromChatID:      pending.ChatID,
		MessageID:       pending.SourceMsgID,
	})

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          h.cfg.TechGroupID,
		MessageThreadID: topic.MessageThreadID,
		Text:            fmt.Sprintf("Linear: %s\nUse /close when done to dump this thread to the issue.", issue.URL),
	})

	filePath := filepath.Join(filepath.Dir(h.cfg.CSVPath), issue.Identifier+".txt")

	tt := &storage.TechThread{
		LinearIssueID:   issue.ID,
		LinearIssueURL:  issue.URL,
		TechChatID:      h.cfg.TechGroupID,
		TechThreadID:    topic.MessageThreadID,
		SourceChatID:    pending.ChatID,
		SourceThreadID:  pending.ThreadID,
		FilePath:        filePath,
		CreatedByUserID: pending.UserID,
	}

	id, err := h.storage.CreateTechThread(ctx, tt)
	if err != nil {
		log.Printf("⚠️  Failed to save tech thread to DB: %v", err)
	}
	tt.ID = id

	h.mu.Lock()
	h.techThreads[techThreadKey(tt.TechChatID, tt.TechThreadID)] = tt
	delete(h.states, stateKey{UserID: pending.UserID})
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    pending.ChatID,
		MessageID: pending.MessageID,
		Text:      fmt.Sprintf("✅ Thread opened!\n\nLinear: %s\nTopic: %s", issue.URL, topicName),
	})
	log.Printf("✓ Tech thread created: %s (topic %d in chat %d)", issue.Identifier, topic.MessageThreadID, h.cfg.TechGroupID)
}

// handleCloseThread closes the current tech thread topic and dumps messages to Linear.
// Must be used inside a tech group topic.
func (h *Handler) handleCloseThread(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if h.cfg.TechGroupID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ TECH_GROUP_ID is not configured.")
		return
	}
	if msg.Chat.ID != h.cfg.TechGroupID || msg.MessageThreadID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ /close must be used inside a tech thread topic.")
		return
	}

	h.mu.Lock()
	tt := h.techThreads[techThreadKey(msg.Chat.ID, msg.MessageThreadID)]
	h.mu.Unlock()

	if tt == nil {
		// Not in cache — try DB (handles the case where bot restarted after thread creation)
		var err error
		tt, err = h.storage.GetTechThreadByTopic(ctx, msg.Chat.ID, msg.MessageThreadID)
		if err != nil {
			h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ DB error: %v", err))
			return
		}
		if tt == nil {
			h.sendMessage(ctx, b, msg, "⚠️ No open thread found for this topic.")
			return
		}
	}

	// Read messages from file
	data, err := os.ReadFile(tt.FilePath)
	if err != nil && !os.IsNotExist(err) {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to read thread file: %v", err))
		return
	}

	// Post comment to Linear
	var comment string
	if len(data) == 0 {
		comment = fmt.Sprintf("## Telegram Thread\n\nClosed by @%s — no messages were logged.", msg.From.Username)
	} else {
		comment = fmt.Sprintf("## Telegram Thread\n\nClosed by @%s on %s\n\n---\n\n%s",
			msg.From.Username,
			time.Now().UTC().Format("2006-01-02 15:04 UTC"),
			string(data))
	}

	if err := h.linear.CreateComment(ctx, tt.LinearIssueID, comment); err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to post to Linear: %v", err))
		return
	}

	// Close the Telegram topic
	b.CloseForumTopic(ctx, &tgbot.CloseForumTopicParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
	})

	// Mark closed in DB
	if err := h.storage.CloseTechThread(ctx, tt.ID); err != nil {
		log.Printf("⚠️  Failed to close tech thread in DB: %v", err)
	}

	// Remove from cache
	h.mu.Lock()
	delete(h.techThreads, techThreadKey(msg.Chat.ID, msg.MessageThreadID))
	h.mu.Unlock()

	// Delete file
	if len(data) > 0 {
		if err := os.Remove(tt.FilePath); err != nil {
			log.Printf("⚠️  Failed to delete thread file %s: %v", tt.FilePath, err)
		}
	}

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            fmt.Sprintf("✅ Thread closed. Messages dumped to Linear: %s", tt.LinearIssueURL),
	})
	log.Printf("✓ Tech thread closed: %s (by @%s)", tt.LinearIssueURL, msg.From.Username)
}

// appendToThreadFile appends a formatted message line to the thread file.
func appendToThreadFile(filePath string, msg *models.Message) {
	if msg.From == nil {
		return
	}
	ts := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02 15:04:05")
	name := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	username := msg.From.Username
	sender := name
	if username != "" {
		sender = fmt.Sprintf("%s (@%s)", name, username)
	}
	content := messageContent(msg)
	line := fmt.Sprintf("[%s] %s: %s\n", ts, sender, content)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("⚠️  thread file open: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		log.Printf("⚠️  thread file write: %v", err)
	}
}
