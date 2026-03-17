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

// handleTicketStart initiates the /ticket flow from a replied-to message.
func (h *Handler) handleTicketStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if msg.ReplyToMessage == nil {
		h.sendMessage(ctx, b, msg, "❌ Reply to a message with /ticket to create a ticket from it.")
		return
	}

	replied := msg.ReplyToMessage

	link := formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, replied.ID)

	isForward := replied.ForwardOrigin != nil
	log.Printf("🎫 /ticket reply target: msg_id=%d, from=%+v, is_forward=%v, forward_origin=%+v",
		replied.ID, replied.From, isForward, replied.ForwardOrigin)

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
		h.sendMessage(ctx, b, msg, "❌ No categories configured. Contact admin.")
		return
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🗂️ Select category for this ticket:",
		ReplyMarkup:     buildCategoryKeyboard(categories),
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingSession{
		Flow:             FlowTicket,
		Step:             StepCategory,
		UserID:           msg.From.ID,
		MessageID:        sentMsg.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		CreatedAt:        time.Now(),
		TicketMsgLink:    link,
		TicketMsgBody:    replied.Text,
		TicketMsgDate:    time.Unix(int64(replied.Date), 0),
		ReporterName:     reporterName,
		ReporterUsername: reporterUsername,
	}
	h.mu.Unlock()

	log.Printf("✓ /ticket started by %s for message from %s (%s)", msg.From.Username, reporterName, reporterUsername)
}

// handleCancelCallback handles the ❌ Cancel inline button press.
func (h *Handler) handleCancelCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	userID := query.From.ID
	key := stateKey{UserID: userID}

	h.mu.Lock()
	_, had := h.states[key]
	delete(h.states, key)
	h.mu.Unlock()

	if !had {
		return
	}

	msg := query.Message.Message
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text:      "❌ Cancelled.",
	})
	log.Printf("✓ Flow cancelled by %s", query.From.Username)
}

// ticketTitle builds a Linear issue title from the first 5 words of the message body and its date.
func ticketTitle(body string, date time.Time) string {
	words := strings.Fields(body)
	if len(words) > 5 {
		words = words[:5]
	}
	snippet := strings.Join(words, " ")
	if len(strings.Fields(body)) > 5 {
		snippet += "..."
	}
	return fmt.Sprintf("%s (%s)", snippet, date.Format("2006-01-02 15:04"))
}

// createTicketIssue creates a Linear issue from the ticket data
func (h *Handler) createTicketIssue(ctx context.Context, b *tgbot.Bot, pending *pendingSession) {
	title := ticketTitle(pending.TicketMsgBody, pending.TicketMsgDate)
	if title == "" || strings.TrimSpace(pending.TicketMsgBody) == "" {
		title = fmt.Sprintf("[%s] Ticket from Telegram (%s)", pending.CategoryName, pending.TicketMsgDate.Format("2006-01-02 15:04"))
	}

	reporter := pending.ReporterName
	if pending.ReporterUsername != "" {
		reporter = fmt.Sprintf("%s (@%s)", pending.ReporterName, pending.ReporterUsername)
	}

	description := fmt.Sprintf("**📌 Telegram Source**\n"+
		"- **Reporter:** %s\n"+
		"- **Category:** %s\n"+
		"- **Type:** %s\n"+
		"- **Link:** %s",
		reporter,
		pending.CategoryName,
		pending.TypeName,
		pending.TicketMsgLink)

	if pending.TicketMsgBody != "" {
		description = fmt.Sprintf("**💬 Message**\n%s\n\n", pending.TicketMsgBody) + description
	}

	// Get on-duty support person
	onDutyResult, err := h.storage.GetOnDutyPersonResult(ctx, pending.CategoryID, time.Now())
	if err != nil {
		log.Printf("⚠️  Failed to get on-duty person: %v", err)
		onDutyResult = nil
	}

	assignee := ""
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignee = onDutyResult.Person.LinearUsername
	}

	if onDutyResult != nil && !onDutyResult.Online {
		description += "\n\n⚠️ **Note:** Assigned person is currently outside working hours."
	}

	url, err := h.linear.CreateIssue(ctx, title, description, pending.TeamKey, assignee, []string{pending.CategoryName, pending.TypeName})
	if err != nil {
		log.Printf("❌ Failed to create Linear issue: %v\n", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to create issue: %v", err),
		})
		return
	}

	h.mu.Lock()
	delete(h.states, stateKey{UserID: pending.UserID})
	h.mu.Unlock()

	assigneeStr := "(unassigned)"
	if onDutyResult != nil && onDutyResult.Person != nil {
		person := onDutyResult.Person
		status := "🟢"
		if !onDutyResult.Online {
			status = "🔴"
		}
		assigneeStr = fmt.Sprintf("%s %s\n  🔵 Telegram: @%s\n  🔷 Linear: @%s", person.Name, status, person.TelegramUsername, person.LinearUsername)
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    pending.ChatID,
		MessageID: pending.MessageID,
		Text: fmt.Sprintf(
			"✅ Ticket created!\n\n"+
				"📋 Category: %s\n"+
				"👤 Assigned to: %s\n"+
				"🔗 Linear: %s",
			pending.CategoryName,
			assigneeStr,
			url,
		),
	})

	assignedPersonName := "unassigned"
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignedPersonName = onDutyResult.Person.Name
	}
	log.Printf("✓ Ticket created: %s (assigned to %v)", url, assignedPersonName)
}
