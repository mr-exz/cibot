package telegram

import (
	"context"
	"fmt"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log"
	"strconv"
	"strings"
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

	rows := make([][]models.InlineKeyboardButton, 0, len(groups)*2)
	for _, g := range groups {
		title := g.Title
		if title == "" {
			title = fmt.Sprintf("chat_%d", g.ChatID)
		}
		status := "⏳"
		if g.Approved {
			status = "✅"
		}
		tz := g.Timezone
		if tz == "" {
			tz = "UTC"
		}
		sb.WriteString(fmt.Sprintf("\n%s %s (TZ: %s)\n", status, title, tz))

		var actionBtn models.InlineKeyboardButton
		if g.Approved {
			actionBtn = models.InlineKeyboardButton{
				Text:         fmt.Sprintf("❌ Disapprove %s", title),
				CallbackData: fmt.Sprintf("disapprove:%d", g.ChatID),
			}
		} else {
			actionBtn = models.InlineKeyboardButton{
				Text:         fmt.Sprintf("✅ Approve %s", title),
				CallbackData: fmt.Sprintf("approve:%d", g.ChatID),
			}
		}
		tzBtn := models.InlineKeyboardButton{
			Text:         "🕐 Set TZ",
			CallbackData: fmt.Sprintf("grptz:sel:%d", g.ChatID),
		}
		rows = append(rows, []models.InlineKeyboardButton{actionBtn, tzBtn})
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

	msg := query.Message.Message
	h.sendGroupsList(ctx, b, msg.Chat.ID, msg.ID)
}

// handleGroupTZCallback handles grptz: callbacks for group timezone management.
// grptz:sel:{chatID}      — show timezone picker for group
// grptz:set:{chatID}:{tz} — save timezone and return to groups list
// grptz:back:{chatID}     — return to groups list without saving
func (h *Handler) handleGroupTZCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	data := strings.TrimPrefix(query.Data, "grptz:")
	msg := query.Message.Message

	switch {
	case strings.HasPrefix(data, "sel:"):
		chatIDStr := strings.TrimPrefix(data, "sel:")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			return
		}
		h.showGroupTZPicker(ctx, b, msg.Chat.ID, msg.ID, chatID)

	case strings.HasPrefix(data, "set:"):
		rest := strings.TrimPrefix(data, "set:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return
		}
		chatID, err := strconv.ParseInt(rest[:idx], 10, 64)
		if err != nil {
			return
		}
		tz := rest[idx+1:]
		if _, err := parseLocation(tz); err != nil {
			log.Printf("⚠️ invalid timezone %q: %v", tz, err)
			return
		}
		if err := h.storage.SetGroupTimezone(ctx, chatID, tz); err != nil {
			log.Printf("❌ SetGroupTimezone %d %q: %v", chatID, tz, err)
			return
		}
		log.Printf("✓ Group %d timezone set to %q by @%s", chatID, tz, query.From.Username)
		h.sendGroupsList(ctx, b, msg.Chat.ID, msg.ID)

	case strings.HasPrefix(data, "back:"):
		h.sendGroupsList(ctx, b, msg.Chat.ID, msg.ID)
	}
}

func (h *Handler) showGroupTZPicker(ctx context.Context, b *tgbot.Bot, adminChatID int64, msgID int, groupChatID int64) {
	tzs, _, _, err := h.storage.GetSupportPersonDefaults(ctx)
	if err != nil {
		log.Printf("❌ GetSupportPersonDefaults: %v", err)
	}

	// Merge with common presets so there's always something to pick even on a fresh install
	presets := []string{"UTC", "+05:00", "+06:00", "+03:00", "+04:00"}
	seen := map[string]bool{}
	for _, t := range tzs {
		seen[t] = true
	}
	for _, p := range presets {
		if !seen[p] {
			tzs = append(tzs, p)
			seen[p] = true
		}
	}

	prefix := fmt.Sprintf("grptz:set:%d:", groupChatID)
	kb := buildPickerKeyboard(tzs, prefix, false)

	// Add a Back button at the bottom
	kb.InlineKeyboard = append(kb.InlineKeyboard, []models.InlineKeyboardButton{{
		Text:         "⬅️ Back",
		CallbackData: fmt.Sprintf("grptz:back:%d", groupChatID),
	}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      adminChatID,
		MessageID:   msgID,
		Text:        "Select timezone for this group:",
		ReplyMarkup: kb,
	})
}
