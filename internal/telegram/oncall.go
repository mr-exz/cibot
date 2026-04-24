package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) handleOnCall(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	categories, err := h.storage.ListCategoriesForContext(ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("❌ /oncall ListCategoriesForContext: %v", err)
		h.sendMessage(ctx, b, msg, "Failed to load categories.")
		return
	}
	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "No support categories configured for this area.")
		return
	}

	now := time.Now()
	var sb strings.Builder
	sb.WriteString("On-duty support:\n\n")

	seen := map[string]bool{}
	var pingUsernames []string

	for _, cat := range categories {
		result, err := h.storage.GetOnDutyPersonResult(ctx, cat.ID, now)
		if err != nil {
			continue
		}
		person := result.Person

		var indicator string
		if person.Status != "" {
			indicator = statusEmoji(person.Status)
		} else if result.Online {
			indicator = "🟢"
		} else {
			indicator = "🔴"
		}

		line := fmt.Sprintf("%s %s: %s (%s) - ", cat.Emoji, cat.Name, person.Name, person.TelegramUsername)
		if person.Status != "" {
			line += person.Status + " " + indicator
		} else if result.Online {
			line += "available " + indicator
		} else {
			line += "offline " + indicator
		}
		if person.WorkHours != "" {
			tz := person.Timezone
			if tz == "" {
				tz = "UTC"
			}
			line += " (hours: " + person.WorkHours + " " + tz + ")"
		}
		sb.WriteString(line + "\n")

		if person.TelegramUsername != "" && !seen[person.TelegramUsername] {
			seen[person.TelegramUsername] = true
			pingUsernames = append(pingUsernames, person.TelegramUsername)
		}
	}

	params := &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   strings.TrimRight(sb.String(), "\n"),
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}
	if len(pingUsernames) > 0 {
		var rows [][]models.InlineKeyboardButton
		for _, u := range pingUsernames {
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: "Ping @" + u, CallbackData: "ping:" + u},
			})
		}
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	b.SendMessage(ctx, params)
}

func (h *Handler) handlePingCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	username := strings.TrimPrefix(query.Data, "ping:")
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	chatID := query.Message.Message.Chat.ID
	threadID := query.Message.Message.MessageThreadID
	params := &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "@" + username,
	}
	if threadID != 0 {
		params.MessageThreadID = threadID
	}
	b.SendMessage(ctx, params)
}

func (h *Handler) handleSetStatus(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if msg.From == nil || msg.From.Username == "" {
		h.sendMessage(ctx, b, msg, "Cannot identify you. Please set a Telegram username first.")
		return
	}

	person, err := h.storage.GetSupportPersonByTelegramUsername(ctx, msg.From.Username)
	if err != nil {
		log.Printf("❌ GetSupportPersonByTelegramUsername @%s: %v", msg.From.Username, err)
		h.sendMessage(ctx, b, msg, "Failed to look up your support profile.")
		return
	}
	if person == nil {
		h.sendMessage(ctx, b, msg, "You are not registered as a support person.")
		return
	}

	now := time.Now()

	// Current availability status
	var statusLine string
	if person.Status != "" {
		statusLine = fmt.Sprintf("Status: %s %s", statusEmoji(person.Status), person.Status)
	} else if isPersonOnlineNow(person, now) {
		statusLine = "Status: 🟢 Available"
	} else {
		statusLine = "Status: 🔴 Offline (outside work hours)"
	}

	// Find categories this person is currently on duty for
	rotations, _ := h.storage.ListAllRotations(ctx, now)
	var onDutyLines []string
	for _, r := range rotations {
		if r.OnDuty != nil && r.OnDuty.ID == person.ID {
			onDutyLines = append(onDutyLines, fmt.Sprintf("  %s %s", r.Category.Emoji, r.Category.Name))
		}
	}

	var sb strings.Builder
	sb.WriteString(statusLine + "\n")
	if len(onDutyLines) > 0 {
		sb.WriteString("\nOn duty now:\n")
		for _, l := range onDutyLines {
			sb.WriteString(l + "\n")
		}
	} else {
		sb.WriteString("\nNot on duty right now.\n")
	}
	sb.WriteString("\nChange status:")

	params := &tgbot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        sb.String(),
		ReplyMarkup: buildStatusKeyboard(),
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}
	b.SendMessage(ctx, params)
}

func (h *Handler) handleStatusCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	status := strings.TrimPrefix(query.Data, "setstatus:")

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	if query.From.Username == "" {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      "Cannot identify you. Please set a Telegram username first.",
		})
		return
	}

	person, err := h.storage.GetSupportPersonByTelegramUsername(ctx, query.From.Username)
	if err != nil {
		log.Printf("❌ handleStatusCallback GetSupportPersonByTelegramUsername @%s: %v", query.From.Username, err)
		return
	}
	if person == nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      "You are not registered as a support person.",
		})
		return
	}

	var reply string

	if status == "back" {
		if err := h.storage.ClearPersonStatus(ctx, person.ID); err != nil {
			log.Printf("❌ ClearPersonStatus for person %d: %v", person.ID, err)
		}
		h.setTagInAllGroups(ctx, query.From.ID, query.From.Username, "")
		reply = "You are back on duty."
	} else {
		if err := h.storage.SetPersonStatus(ctx, person.ID, status); err != nil {
			log.Printf("❌ SetPersonStatus for person %d: %v", person.ID, err)
		}
		h.setTagInAllGroups(ctx, query.From.ID, query.From.Username, statusTag(status))
		reply = "Status set to: " + statusTag(status) + ". Use /status back when you return."
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    query.Message.Message.Chat.ID,
		MessageID: query.Message.Message.ID,
		Text:      reply,
	})
}

func (h *Handler) setTagInAllGroups(ctx context.Context, userID int64, username string, tag string) {
	groupIDs, err := h.storage.ListApprovedGroupIDs(ctx)
	if err != nil {
		log.Printf("❌ setTagInAllGroups ListApprovedGroupIDs: %v", err)
		return
	}
	for _, groupID := range groupIDs {
		if err := setChatMemberTag(ctx, h.cfg.TelegramToken, groupID, userID, tag); err != nil {
			log.Printf("⚠️ setChatMemberTag group %d for @%s: %v", groupID, username, err)
		}
	}
}

func buildStatusKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🍽 Lunch", CallbackData: "setstatus:lunch"},
				{Text: "⏸ BRB", CallbackData: "setstatus:brb"},
			},
			{
				{Text: "🚫 Away", CallbackData: "setstatus:away"},
				{Text: "🟢 Back", CallbackData: "setstatus:back"},
			},
		},
	}
}

func statusEmoji(status string) string {
	switch status {
	case "lunch":
		return "🍽"
	case "brb":
		return "⏸"
	case "away":
		return "🚫"
	default:
		return "❓"
	}
}

func statusTag(status string) string {
	switch status {
	case "lunch":
		return "On lunch"
	case "brb":
		return "BRB"
	case "away":
		return "Away"
	default:
		return ""
	}
}
