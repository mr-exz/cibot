package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/storage"
)

func (h *Handler) handleTakeover(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.startAdminCategoryPicker(ctx, b, msg, AdminCmdTakeover)
}

// handleTakeoverCategorySelected is called when a category is selected in the takeover flow.
func (h *Handler) handleTakeoverCategorySelected(ctx context.Context, b *tgbot.Bot, admin *pendingAdminSession, cat *storage.Category) {
	// Get all support persons
	persons, err := h.storage.ListAllSupportPersons(ctx)
	if err != nil {
		log.Printf("❌ ListAllSupportPersons: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("❌ Failed to load persons: %v", err),
		})
		return
	}

	if len(persons) == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      "❌ No support persons available.",
		})
		return
	}

	// Build person keyboard
	rows := make([][]models.InlineKeyboardButton, len(persons))
	for i, p := range persons {
		rows[i] = []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s (@%s)", p.Name, p.TelegramUsername),
			CallbackData: fmt.Sprintf("takeover:person:%d:%d", cat.ID, p.ID),
		}}
	}

	admin.Step = StepAdminTakeoverPerson
	h.mu.Lock()
	h.states[stateKey{UserID: admin.UserID}] = admin
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        fmt.Sprintf("✓ Category: %s %s\n\n👤 Who is taking over?", cat.Emoji, cat.Name),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (h *Handler) handleTakeoverCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	data := strings.TrimPrefix(query.Data, "takeover:")
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	messageID := msg.ID
	userID := query.From.ID

	switch {
	case strings.HasPrefix(data, "person:"):

	case strings.HasPrefix(data, "person:"):
		// Person selected
		parts := strings.SplitN(strings.TrimPrefix(data, "person:"), ":", 2)
		if len(parts) != 2 {
			return
		}

		categoryID, err1 := strconv.ParseInt(parts[0], 10, 64)
		personID, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}

		cat, _ := h.storage.GetCategory(ctx, categoryID)
		persons, _ := h.storage.ListAllSupportPersons(ctx)
		var person *models.User
		for i := range persons {
			if persons[i].ID == personID {
				person = &models.User{
					ID:        persons[i].ID,
					FirstName: persons[i].Name,
					Username:  persons[i].TelegramUsername,
				}
				break
			}
		}

		if person == nil {
			return
		}

		endOfWeek := time.Now().AddDate(0, 0, (5 - int(time.Now().Weekday()) + 7) % 7)
		if endOfWeek.Equal(time.Now()) {
			endOfWeek = endOfWeek.AddDate(0, 0, 1)
		}
		endOfWeekStr := endOfWeek.Format("2006-01-02")

		rows := [][]models.InlineKeyboardButton{
			{
				{
					Text:         "🕐 Today only",
					CallbackData: fmt.Sprintf("takeover:duration:%d:%d:today", categoryID, personID),
				},
			},
			{
				{
					Text:         "📅 Until Friday",
					CallbackData: fmt.Sprintf("takeover:duration:%d:%d:friday", categoryID, personID),
				},
			},
			{
				{
					Text:         "📆 This week",
					CallbackData: fmt.Sprintf("takeover:duration:%d:%d:%s", categoryID, personID, endOfWeekStr),
				},
			},
			{
				{
					Text:         "❌ Cancel",
					CallbackData: "takeover:start",
				},
			},
		}

		text := fmt.Sprintf("✓ Category: %s %s\n✓ Person: @%s\n\n⏱️ How long?", cat.Emoji, cat.Name, person.Username)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})

	case strings.HasPrefix(data, "duration:"):
		// Duration selected
		parts := strings.SplitN(strings.TrimPrefix(data, "duration:"), ":", 3)
		if len(parts) < 3 {
			return
		}

		categoryID, _ := strconv.ParseInt(parts[0], 10, 64)
		personID, _ := strconv.ParseInt(parts[1], 10, 64)

		fromDate := time.Now().Format("2006-01-02")
		untilDate := fromDate

		switch parts[2] {
		case "today":
			untilDate = fromDate
		case "friday":
			endOfWeek := time.Now().AddDate(0, 0, (5 - int(time.Now().Weekday()) + 7) % 7)
			if endOfWeek.Before(time.Now()) {
				endOfWeek = endOfWeek.AddDate(0, 0, 7)
			}
			untilDate = endOfWeek.Format("2006-01-02")
		default:
			// Assume it's a date string
			untilDate = parts[2]
		}

		cat, _ := h.storage.GetCategory(ctx, categoryID)
		persons, _ := h.storage.ListAllSupportPersons(ctx)
		var personName string
		for i := range persons {
			if persons[i].ID == personID {
				personName = persons[i].Name
				break
			}
		}

		// Confirm
		rows := [][]models.InlineKeyboardButton{
			{
				{
					Text:         "✅ Confirm",
					CallbackData: fmt.Sprintf("takeover:confirm:%d:%d:%s:%s", categoryID, personID, fromDate, untilDate),
				},
				{
					Text:         "❌ Cancel",
					CallbackData: "takeover:start",
				},
			},
		}

		text := fmt.Sprintf(
			"✓ Category: %s %s\n✓ Person: %s\n✓ From: %s\n✓ Until: %s\n\n✅ Confirm?",
			cat.Emoji, cat.Name, personName, fromDate, untilDate,
		)

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})

	case strings.HasPrefix(data, "confirm:"):
		// Confirm and set takeover
		parts := strings.SplitN(strings.TrimPrefix(data, "confirm:"), ":", 4)
		if len(parts) != 4 {
			return
		}

		categoryID, _ := strconv.ParseInt(parts[0], 10, 64)
		personID, _ := strconv.ParseInt(parts[1], 10, 64)
		fromDate := parts[2]
		untilDate := parts[3]

		setBy := query.From.Username
		if setBy == "" {
			setBy = query.From.FirstName
		}

		if err := h.storage.SetTakeover(ctx, categoryID, personID, setBy, fromDate, untilDate); err != nil {
			log.Printf("❌ SetTakeover: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: messageID,
				Text:      fmt.Sprintf("❌ Failed to set takeover: %v", err),
			})
			return
		}

		h.mu.Lock()
		delete(h.states, stateKey{UserID: userID})
		h.mu.Unlock()

		rows := [][]models.InlineKeyboardButton{
			{
				{
					Text:         "🔄 Clear takeover",
					CallbackData: fmt.Sprintf("takeover:clear:%d", categoryID),
				},
			},
		}

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        fmt.Sprintf("✅ Takeover set from %s to %s", fromDate, untilDate),
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})

		log.Printf("✓ Takeover set: category %d → person %d, %s–%s (by @%s)", categoryID, personID, fromDate, untilDate, setBy)

	case strings.HasPrefix(data, "clear:"):
		categoryID, err := strconv.ParseInt(strings.TrimPrefix(data, "clear:"), 10, 64)
		if err != nil {
			return
		}

		if err := h.storage.ClearTakeover(ctx, categoryID); err != nil {
			log.Printf("❌ ClearTakeover: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: messageID,
				Text:      fmt.Sprintf("❌ Failed to clear takeover: %v", err),
			})
			return
		}

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
			Text:      "✅ Takeover cleared. Rotation resumed.",
		})

		log.Printf("✓ Takeover cleared: category %d", categoryID)
	}
}
