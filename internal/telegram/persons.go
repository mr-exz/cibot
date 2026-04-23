package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/storage"
)

func (h *Handler) handlePersonsCommand(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.showPersonsList(ctx, b, msg.Chat.ID, 0)
}

func (h *Handler) showPersonsList(ctx context.Context, b *tgbot.Bot, chatID int64, messageID int) {
	persons, err := h.storage.ListAllSupportPersons(ctx)
	if err != nil {
		log.Printf("❌ /persons ListAllSupportPersons: %v", err)
		return
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(persons)+1)
	for _, p := range persons {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s (@%s)", p.Name, p.TelegramUsername),
			CallbackData: fmt.Sprintf("pmgr:view:%d", p.ID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         "➕ Add person",
		CallbackData: "pmgr:addperson",
	}})
	kb := &models.InlineKeyboardMarkup{InlineKeyboard: rows}

	text := "Support persons:"
	if len(persons) == 0 {
		text = "No support persons yet."
	}
	if messageID != 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: kb,
		})
	} else {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        text,
			ReplyMarkup: kb,
		})
	}
}

func (h *Handler) showPersonDetail(ctx context.Context, b *tgbot.Bot, chatID int64, messageID int, personID int64) {
	persons, err := h.storage.ListAllSupportPersons(ctx)
	if err != nil {
		log.Printf("❌ showPersonDetail ListAllSupportPersons: %v", err)
		return
	}
	var person *storage.SupportPerson
	for i := range persons {
		if persons[i].ID == personID {
			person = &persons[i]
			break
		}
	}
	if person == nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
			Text:      "Person not found.",
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "⬅️ Back", CallbackData: "pmgr:list"}},
			}},
		})
		return
	}

	cats, err := h.storage.ListCategoriesForPerson(ctx, personID)
	if err != nil {
		log.Printf("❌ showPersonDetail ListCategoriesForPerson: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 %s (@%s)\n", person.Name, person.TelegramUsername))
	sb.WriteString(fmt.Sprintf("🔷 Linear: @%s\n", person.LinearUsername))

	tz := person.Timezone
	if tz == "" {
		tz = "—"
	}
	hours := person.WorkHours
	if hours == "" {
		hours = "—"
	}
	days := person.WorkDays
	if days == "" {
		days = "—"
	}
	sb.WriteString(fmt.Sprintf("🌍 %s  ⏰ %s  📅 %s\n", tz, hours, days))

	if len(cats) == 0 {
		sb.WriteString("\nNot assigned to any category.")
	} else {
		sb.WriteString("\nAssigned to:")
		for _, c := range cats {
			sb.WriteString(fmt.Sprintf("\n  %s %s", c.Emoji, c.Name))
		}
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(cats)+4)
	for _, c := range cats {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("🚫 Remove from %s %s", c.Emoji, c.Name),
			CallbackData: fmt.Sprintf("pmgr:rmcat:%d:%d", personID, c.ID),
		}})
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{
			Text:         "➕ Add to category",
			CallbackData: fmt.Sprintf("pmgr:addtocat:%d", personID),
		}},
		[]models.InlineKeyboardButton{{
			Text:         "✏️ Edit schedule",
			CallbackData: fmt.Sprintf("pmgr:editsch:%d", personID),
		}},
		[]models.InlineKeyboardButton{{
			Text:         "🗑 Delete person",
			CallbackData: fmt.Sprintf("pmgr:del:%d", personID),
		}},
		[]models.InlineKeyboardButton{{Text: "⬅️ Back", CallbackData: "pmgr:list"}},
	)

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        sb.String(),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (h *Handler) handlePersonsCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	data := strings.TrimPrefix(query.Data, "pmgr:")
	chatID := query.Message.Message.Chat.ID
	messageID := query.Message.Message.ID
	userID := query.From.ID

	switch {
	case data == "list":
		h.showPersonsList(ctx, b, chatID, messageID)

	case strings.HasPrefix(data, "view:"):
		personID, err := strconv.ParseInt(strings.TrimPrefix(data, "view:"), 10, 64)
		if err != nil {
			return
		}
		h.showPersonDetail(ctx, b, chatID, messageID, personID)

	case data == "addperson":
		h.startAdminCategoryPickerInline(ctx, b, &pendingAdminSession{
			Cmd:       AdminCmdAddPerson,
			ChatID:    chatID,
			MessageID: messageID,
			UserID:    userID,
		})

	case strings.HasPrefix(data, "addtocat:"):
		personID, err := strconv.ParseInt(strings.TrimPrefix(data, "addtocat:"), 10, 64)
		if err != nil {
			return
		}
		h.startAdminCategoryPickerInline(ctx, b, &pendingAdminSession{
			Cmd:       AdminCmdAddPersonToCategory,
			ChatID:    chatID,
			MessageID: messageID,
			UserID:    userID,
			PersonID:  personID,
		})

	case strings.HasPrefix(data, "editsch:"):
		personID, err := strconv.ParseInt(strings.TrimPrefix(data, "editsch:"), 10, 64)
		if err != nil {
			return
		}
		persons, _ := h.storage.ListAllSupportPersons(ctx)
		var person *storage.SupportPerson
		for i := range persons {
			if persons[i].ID == personID {
				person = &persons[i]
				break
			}
		}
		if person == nil {
			return
		}
		admin := &pendingAdminSession{
			Cmd:       AdminCmdSetWorkHours,
			Step:      StepAdminWhTimezone,
			ChatID:    chatID,
			MessageID: messageID,
			UserID:    userID,
			PersonID:  personID,
			TgUsername: person.TelegramUsername,
			Timezone:  person.Timezone,
			WorkHours: person.WorkHours,
			WorkDays:  person.WorkDays,
		}
		h.mu.Lock()
		h.states[stateKey{UserID: userID}] = admin
		h.mu.Unlock()
		h.showTzPicker(ctx, b, admin, true)

	case strings.HasPrefix(data, "rmcat:"):
		parts := strings.SplitN(strings.TrimPrefix(data, "rmcat:"), ":", 2)
		if len(parts) != 2 {
			return
		}
		personID, err1 := strconv.ParseInt(parts[0], 10, 64)
		categoryID, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}
		if err := h.storage.RemovePersonFromCategory(ctx, personID, categoryID); err != nil {
			log.Printf("❌ RemovePersonFromCategory person=%d cat=%d: %v", personID, categoryID, err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: messageID,
				Text:      fmt.Sprintf("❌ Failed to remove: %v", err),
			})
			return
		}
		h.showPersonDetail(ctx, b, chatID, messageID, personID)

	case strings.HasPrefix(data, "del:") && !strings.HasPrefix(data, "delconf:"):
		personID, err := strconv.ParseInt(strings.TrimPrefix(data, "del:"), 10, 64)
		if err != nil {
			return
		}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
			Text:      "Delete this person? They will be removed from all categories permanently.",
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "✓ Confirm delete", CallbackData: fmt.Sprintf("pmgr:delconf:%d", personID)},
					{Text: "✗ Cancel", CallbackData: fmt.Sprintf("pmgr:view:%d", personID)},
				},
			}},
		})

	case strings.HasPrefix(data, "delconf:"):
		personID, err := strconv.ParseInt(strings.TrimPrefix(data, "delconf:"), 10, 64)
		if err != nil {
			return
		}
		if err := h.storage.DeleteSupportPerson(ctx, personID); err != nil {
			log.Printf("❌ DeleteSupportPerson %d: %v", personID, err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: messageID,
				Text:      fmt.Sprintf("❌ Failed to delete: %v", err),
			})
			return
		}
		h.showPersonsList(ctx, b, chatID, messageID)
	}
}
