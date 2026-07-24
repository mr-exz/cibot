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

// handleRearrange starts the /rearrange flow: pick a category, preview the
// schedule regeneration would produce, confirm.
func (h *Handler) handleRearrange(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.startAdminCategoryPicker(ctx, b, msg, AdminCmdRearrange)
}

// handleRearrangeCategorySelected shows the stored future schedule next to the
// schedule that regeneration would produce, and asks for confirmation.
func (h *Handler) handleRearrangeCategorySelected(ctx context.Context, b *tgbot.Bot, admin *pendingAdminSession, cat *storage.Category) {
	now := time.Now()

	after, err := h.storage.PreviewRegeneratedTurns(ctx, cat.ID, now)
	if err != nil {
		log.Printf("❌ PreviewRegeneratedTurns category %d: %v", cat.ID, err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf(h.trans.Error.Failed, err),
		})
		return
	}
	if len(after) == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      h.trans.Person.NoPersonsAvailable,
		})
		return
	}

	before, err := h.storage.ListScheduledTurns(ctx, cat.ID, now)
	if err != nil {
		log.Printf("❌ ListScheduledTurns category %d: %v", cat.ID, err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf(h.trans.Error.Failed, err),
		})
		return
	}

	persons, err := h.storage.ListAllSupportPersons(ctx)
	if err != nil {
		log.Printf("❌ ListAllSupportPersons: %v", err)
		persons = nil
	}
	byID := personsByID(persons)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s — rearrange rotation\n", cat.Emoji, cat.Name))
	sb.WriteString("\nCurrent schedule:\n")
	sb.WriteString(formatScheduleTurns(before, byID, now, "  "))
	sb.WriteString("\nAfter rearrange:\n")
	sb.WriteString(formatScheduleTurns(after, byID, now, "  "))
	sb.WriteString("\nPast days and today are kept as they are. Apply?")

	admin.Step = StepAdminRearrangeConfirm
	h.mu.Lock()
	h.states[stateKey{UserID: admin.UserID}] = admin
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    admin.ChatID,
		MessageID: admin.MessageID,
		Text:      sb.String(),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.trans.Admin.Confirm, CallbackData: fmt.Sprintf("rearr:apply:%d", cat.ID)},
				{Text: h.trans.Admin.Cancel, CallbackData: "cancel"},
			},
		}},
	})
}

// handleRearrangeCallback applies the previewed regeneration after confirmation.
func (h *Handler) handleRearrangeCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	// Guard against stale/replayed callbacks: only act while a rearrange
	// confirmation for this category is pending.
	h.mu.Lock()
	state, ok := h.states[stateKey{UserID: query.From.ID}]
	h.mu.Unlock()
	admin, isAdminSession := state.(*pendingAdminSession)
	if !ok || !isAdminSession || admin.Cmd != AdminCmdRearrange || admin.Step != StepAdminRearrangeConfirm {
		return
	}

	categoryID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "rearr:apply:"), 10, 64)
	if err != nil || categoryID != admin.CategoryID {
		return
	}

	now := time.Now()
	if err := h.storage.RegenerateUpcomingTurns(ctx, categoryID, now); err != nil {
		log.Printf("❌ RegenerateUpcomingTurns category %d: %v", categoryID, err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      fmt.Sprintf(h.trans.Error.Failed, err),
		})
		return
	}

	h.mu.Lock()
	delete(h.states, stateKey{UserID: query.From.ID})
	h.mu.Unlock()

	turns, _ := h.storage.ListScheduledTurns(ctx, categoryID, now)
	persons, _ := h.storage.ListAllSupportPersons(ctx)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ Rotation rearranged for %s.\n\nNew schedule:\n", admin.CategoryName))
	sb.WriteString(formatScheduleTurns(turns, personsByID(persons), now, "  "))

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text:      strings.TrimRight(sb.String(), "\n"),
	})
	log.Printf("✓ Rotation rearranged for category %d by @%s", categoryID, query.From.Username)
}

func personsByID(persons []storage.SupportPerson) map[int64]storage.SupportPerson {
	m := make(map[int64]storage.SupportPerson, len(persons))
	for _, p := range persons {
		m[p.ID] = p
	}
	return m
}

// formatScheduleTurns renders materialized turns one per line, marking the turn
// that covers today. Each line is prefixed with indent.
func formatScheduleTurns(turns []storage.ScheduleTurn, persons map[int64]storage.SupportPerson, now time.Time, indent string) string {
	if len(turns) == 0 {
		return indent + "(nothing scheduled)\n"
	}

	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	current := -1
	for i, t := range turns {
		if !t.Start.After(today) {
			current = i
		}
	}

	var sb strings.Builder
	for i, t := range turns {
		label := fmt.Sprintf("person %d", t.PersonID)
		if p, ok := persons[t.PersonID]; ok {
			label = fmt.Sprintf("%s (@%s)", p.Name, p.TelegramUsername)
		}
		line := fmt.Sprintf("%s%s — %s", indent, t.Start.Format("Mon 02.01"), label)
		if i == current {
			line += " <- today"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
