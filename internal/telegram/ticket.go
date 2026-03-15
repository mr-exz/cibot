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

// handleTicketStart initiates the /ticket flow by asking for a message link.
func (h *Handler) handleTicketStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	log.Printf("✓ Processing /ticket command from chat_id: %d\n", msg.Chat.ID)

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🔗 Paste the Telegram message link:",
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingSession{
		Flow:      FlowTicket,
		Step:      StepTicketLink,
		UserID:    msg.From.ID,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()
}

// handleTicketPendingLink handles the pasted link and advances to category selection.
func (h *Handler) handleTicketPendingLink(ctx context.Context, b *tgbot.Bot, msg *models.Message, pending *pendingSession) {
	link := strings.TrimSpace(msg.Text)

	if !strings.HasPrefix(link, "https://t.me/") {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      "❌ That doesn't look like a Telegram link. Paste a link starting with https://t.me/",
		})
		return
	}

	// Delete the user's message containing the link to keep chat clean
	b.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	})

	categories, err := h.storage.ListCategoriesForContext(ctx, pending.ChatID, pending.ThreadID)
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to load categories: %v", err),
		})
		return
	}
	if len(categories) == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      "❌ No categories configured. Contact admin.",
		})
		return
	}

	pending.TicketMsgLink = link
	pending.Step = StepCategory

	h.mu.Lock()
	h.states[stateKey{UserID: msg.From.ID}] = pending
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      pending.ChatID,
		MessageID:   pending.MessageID,
		Text:        "🗂️ Select category for this ticket:",
		ReplyMarkup: buildCategoryKeyboard(categories),
	})

	log.Printf("✓ Ticket flow: link received %s", link)
}

// handleTicketCategoryCallback handles category selection in ticket flow
func (h *Handler) handleTicketCategoryCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	msg := query.Message.Message
	if msg == nil {
		return
	}

	// Parse category ID from callback data
	categoryID := strings.TrimPrefix(query.Data, "cat:")

	// Answer callback
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "Loading...",
	})

	// Get category details
	cat, err := h.storage.GetCategory(ctx, parseCategoryID(categoryID))
	if err != nil {
		log.Printf("❌ Failed to get category: %v\n", err)
		return
	}

	// Get request types for this category
	types, err := h.storage.ListRequestTypesForCategory(ctx, cat.ID)
	if err != nil {
		log.Printf("❌ Failed to get request types: %v\n", err)
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	pendingIface := h.states[key]
	if pendingIface == nil {
		h.mu.Unlock()
		return
	}

	pending, ok := pendingIface.(*pendingSession)
	if !ok || pending.Flow != FlowTicket {
		h.mu.Unlock()
		return
	}

	pending.CategoryID = cat.ID
	pending.CategoryName = cat.Name
	pending.TeamKey = cat.LinearTeamKey

	// Edit message to ask for request type
	if len(types) == 0 {
		// No request types, create issue immediately
		h.mu.Unlock()
		h.createTicketIssue(ctx, b, pending)
	} else {
		// Show request type keyboard
		pending.Step = StepRequestType
		h.mu.Unlock()

		keyboard := buildRequestTypeKeyboard(types)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      pending.ChatID,
			MessageID:   pending.MessageID,
			Text:        fmt.Sprintf("✓ %s selected.\n\n📋 **Select request type:**", cat.Emoji+" "+cat.Name),
			ReplyMarkup: keyboard,
		})
	}
}

// handleTicketTypeCallback handles request type selection in ticket flow
func (h *Handler) handleTicketTypeCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	msg := query.Message.Message
	if msg == nil {
		return
	}

	// Parse type ID
	typeID := strings.TrimPrefix(query.Data, "type:")

	// Answer callback
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ Creating ticket...",
	})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	pendingIface := h.states[key]
	if pendingIface == nil {
		h.mu.Unlock()
		return
	}

	pending, ok := pendingIface.(*pendingSession)
	if !ok || pending.Flow != FlowTicket {
		h.mu.Unlock()
		return
	}

	pending.TypeID = parseTypeID(typeID)
	h.mu.Unlock()

	// Create the issue immediately
	h.createTicketIssue(ctx, b, pending)
}

// createTicketIssue creates a Linear issue from the ticket data
func (h *Handler) createTicketIssue(ctx context.Context, b *tgbot.Bot, pending *pendingSession) {
	title := fmt.Sprintf("[%s] Ticket from Telegram", pending.CategoryName)

	description := fmt.Sprintf("**📌 Telegram Source**\n"+
		"- **Link:** %s\n"+
		"- **Category:** %s\n"+
		"- **Type:** %s",
		pending.TicketMsgLink,
		pending.CategoryName,
		pending.TypeName)

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

	// Add offline warning to description if person is outside working hours
	if onDutyResult != nil && !onDutyResult.Online {
		description += "\n\n⚠️ **Note:** Assigned person is currently outside working hours."
	}

	// Create Linear issue with category and type as labels
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

	// Clean up state
	h.mu.Lock()
	delete(h.states, stateKey{UserID: pending.UserID})
	h.mu.Unlock()

	// Build final response
	assigneeStr := "(unassigned)"
	if onDutyResult != nil && onDutyResult.Person != nil {
		person := onDutyResult.Person
		status := "🟢"
		if !onDutyResult.Online {
			status = "🔴"
		}
		assigneeStr = fmt.Sprintf("%s %s\n  🔵 Telegram: @%s\n  🔷 Linear: @%s", person.Name, status, person.TelegramUsername, person.LinearUsername)
	}

	finalText := fmt.Sprintf(
		"✅ Ticket created!\n\n"+
			"📋 Category: %s\n"+
			"👤 Assigned to: %s\n"+
			"🔗 Linear: %s",
		pending.CategoryName,
		assigneeStr,
		url,
	)

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    pending.ChatID,
		MessageID: pending.MessageID,
		Text:      finalText,
	})

	assignedPersonName := "unassigned"
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignedPersonName = onDutyResult.Person.Name
	}
	log.Printf("✓ Ticket created: %s (assigned to %v)", url, assignedPersonName)
}
