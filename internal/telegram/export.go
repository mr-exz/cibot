package telegram

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/msglog"
)

// logMessage captures a message to the CSV log.
func (h *Handler) logMessage(msg *models.Message) {
	if msg.From == nil {
		return
	}

	topicName := ""
	if msg.MessageThreadID != 0 {
		topics := h.getTopics(msg.Chat.ID)
		topicName = topics[msg.MessageThreadID]
	}

	entry := msglog.Entry{
		Timestamp: time.Now().UTC(),
		ChatID:    msg.Chat.ID,
		ChatTitle: msg.Chat.Title,
		ChatType:  string(msg.Chat.Type),
		ThreadID:  msg.MessageThreadID,
		TopicName: topicName,
		UserID:    msg.From.ID,
		Username:  msg.From.Username,
		FirstName: msg.From.FirstName,
		LastName:  msg.From.LastName,
		MessageID: msg.ID,
		Text:      messageContent(msg),
	}

	if err := h.msglog.Log(entry); err != nil {
		log.Printf("⚠️  msglog: %v", err)
	}
}

// messageContent returns the text of a message or a media type placeholder.
func messageContent(msg *models.Message) string {
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Photo != nil {
		return "[photo]"
	}
	if msg.Video != nil {
		return "[video]"
	}
	if msg.Document != nil {
		return "[document]"
	}
	if msg.Audio != nil {
		return "[audio]"
	}
	if msg.Voice != nil {
		return "[voice]"
	}
	if msg.VideoNote != nil {
		return "[video_note]"
	}
	if msg.Sticker != nil {
		return "[sticker]"
	}
	if msg.Animation != nil {
		return "[animation]"
	}
	if msg.Location != nil {
		return "[location]"
	}
	if msg.Contact != nil {
		return "[contact]"
	}
	if msg.Poll != nil {
		return "[poll]"
	}
	return "[media]"
}

// handleExport sends the current CSV to the admin and truncates it.
func (h *Handler) handleExport(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	data, err := h.msglog.ExportAndTruncate()
	if err != nil {
		log.Printf("❌ export: %v", err)
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Export failed: %v", err))
		return
	}

	filename := fmt.Sprintf("messages_%s.csv", time.Now().UTC().Format("2006-01-02_150405"))

	_, err = b.SendDocument(ctx, &tgbot.SendDocumentParams{
		ChatID: msg.Chat.ID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(data),
		},
		Caption: fmt.Sprintf("📊 Message log exported — %d bytes\nLog has been reset.", len(data)),
	})
	if err != nil {
		log.Printf("❌ SendDocument: %v", err)
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to send file: %v", err))
		return
	}

	log.Printf("✓ Message log exported by @%s (%d bytes), log truncated", msg.From.Username, len(data))
}
