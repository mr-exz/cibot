package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) handleGroups(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendGroupsList(ctx, b, msg.Chat.ID, 0)
}

func (h *Handler) sendGroupsList(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int) {
	groups, err := h.storage.ListGroups(ctx)
	if err != nil {
		log.Printf("❌ ListGroups: %v", err)
		return
	}

	if len(groups) == 0 {
		text := "No groups registered yet. The bot will auto-register groups when it receives messages."
		if editMsgID != 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{ChatID: chatID, MessageID: editMsgID, Text: text})
		} else {
			b.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatID, Text: text})
		}
		return
	}

	approved, pending := 0, 0
	for _, g := range groups {
		if g.Approved {
			approved++
		} else {
			pending++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Groups (%d approved, %d pending):\n", approved, pending))

	rows := make([][]models.InlineKeyboardButton, 0, len(groups))
	for _, g := range groups {
		title := g.Title
		if title == "" {
			title = fmt.Sprintf("chat_%d", g.ChatID)
		}
		status := "⏳"
		if g.Approved {
			status = "✅"
		}
		sb.WriteString(fmt.Sprintf("\n%s %s\n", status, title))

		if g.Approved {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         fmt.Sprintf("❌ Disapprove %s", title),
				CallbackData: fmt.Sprintf("disapprove:%d", g.ChatID),
			}})
		} else {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         fmt.Sprintf("✅ Approve %s", title),
				CallbackData: fmt.Sprintf("approve:%d", g.ChatID),
			}})
		}
	}

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: rows}

	if editMsgID != 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   editMsgID,
			Text:        sb.String(),
			ReplyMarkup: keyboard,
		})
	} else {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        sb.String(),
			ReplyMarkup: keyboard,
		})
	}
}

func (h *Handler) handleGroupApproveCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	approve := strings.HasPrefix(query.Data, "approve:")
	chatIDStr := strings.TrimPrefix(query.Data, "approve:")
	chatIDStr = strings.TrimPrefix(chatIDStr, "disapprove:")

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return
	}

	if err := h.storage.SetGroupApproved(ctx, chatID, approve); err != nil {
		log.Printf("❌ SetGroupApproved: %v", err)
		return
	}

	action := "approved"
	if !approve {
		action = "disapproved"
	}
	log.Printf("✓ Group %d %s by @%s", chatID, action, query.From.Username)

	// Refresh the list in place
	msg := query.Message.Message
	h.sendGroupsList(ctx, b, msg.Chat.ID, msg.ID)
}
