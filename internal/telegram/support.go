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
)

// handleSupportStart initiates the /support flow, or the /ticket flow if msg is a reply.
func (h *Handler) handleSupportStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if msg.ReplyToMessage != nil {
		h.handleTicketStart(ctx, b, msg)
		return
	}

	log.Printf("✓ Processing /support command from chat_id: %d\n", msg.Chat.ID)

	// Load categories from DB (topic-aware: show global + topic-specific categories)
	categories, err := h.storage.ListCategoriesForContext(ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}

	if len(categories) == 0 {
		if msg.Chat.Type == "private" {
			h.sendMessage(ctx, b, msg, "⚠️ /ticket must be used in a group chat, not here in DM.")
		} else {
			h.sendMessage(ctx, b, msg, h.buildUnconfiguredTopicMsg(ctx, msg.Chat.ID))
		}
		return
	}

	linearUsername, _ := h.storage.GetUserLinearUsername(ctx, msg.From.ID)
	reporterName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)

	text := "📝 Describe your issue (you can also attach a photo or file):"
	if linearUsername == "" {
		text = "⚠️ Your Telegram account is not linked to Linear. Use /mylinear to link it.\n\n" + text
	}

	session := &pendingSession{
		Flow:             FlowSupport,
		Step:             StepDescription,
		UserID:           msg.From.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		CreatedAt:        time.Now(),
		ReporterName:     reporterName,
		ReporterUsername: msg.From.Username,
		RequesterLinear:  linearUsername,
		ChatTitle:        msg.Chat.Title,
	}

	params := &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   text,
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}

	sentMsg, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	session.MessageID = sentMsg.ID
	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = session
	h.mu.Unlock()
}

// handleMyLinear handles /mylinear — lets a user set or update their Linear username.
func (h *Handler) handleMyLinear(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	current, _ := h.storage.GetUserLinearUsername(ctx, msg.From.ID)

	text := "👤 Enter your Linear username:"
	if current != "" {
		text = "👤 Current Linear account: " + current + "\n\nEnter a new username to update:"
	}

	params := &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   text,
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}

	sentMsg, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingSession{
		Flow:      FlowUpdateLinear,
		Step:      StepLinearAccount,
		UserID:    msg.From.ID,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()
}

// handleCategoryCallback handles category selection
func (h *Handler) handleCategoryCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	// Access the Message field
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

	// Handle admin flows (addtype, addperson, setrotation)
	if adminPending, ok := pendingIface.(*pendingAdminSession); ok {
		h.mu.Unlock()
		h.handleAdminCategoryCallback(ctx, b, adminPending, cat)
		return
	}

	pending, ok := pendingIface.(*pendingSession)
	if !ok || pending.Step != StepCategory {
		h.mu.Unlock()
		return
	}

	pending.CategoryID = cat.ID
	pending.CategoryName = cat.Name
	pending.TeamKey = cat.LinearTeamKey

	catLabel := cat.Emoji + " " + cat.Name

	if len(types) == 0 {
		// No request types — skip to priority
		pending.Step = StepPriority
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      pending.ChatID,
			MessageID:   pending.MessageID,
			Text:        "✓ " + catLabel + "\n\nSelect priority:",
			ReplyMarkup: buildPriorityKeyboard(),
		})
	} else {
		// Show request type keyboard
		pending.Step = StepRequestType
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      pending.ChatID,
			MessageID:   pending.MessageID,
			Text:        "✓ " + catLabel + "\n\n📋 Select request type:",
			ReplyMarkup: buildRequestTypeKeyboard(types),
		})
	}
}

// handleRequestTypeCallback handles request type selection
func (h *Handler) handleRequestTypeCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
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
		Text:            "✓ Selected",
	})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	pendingIface := h.states[key]
	if pendingIface == nil {
		h.mu.Unlock()
		return
	}

	pending, ok := pendingIface.(*pendingSession)
	if !ok || pending.Step != StepRequestType {
		h.mu.Unlock()
		return
	}

	pending.TypeID = parseTypeID(typeID)
	h.mu.Unlock()

	// Resolve type name from DB
	if rt, err := h.storage.GetRequestType(ctx, pending.TypeID); err == nil {
		pending.TypeName = rt.Name
	} else {
		log.Printf("⚠️  Failed to resolve type name for ID %d: %v", pending.TypeID, err)
	}

	h.mu.Lock()
	pending.Step = StepPriority
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      pending.ChatID,
		MessageID:   pending.MessageID,
		Text:        "✓ " + pending.TypeName + "\n\nSelect priority:",
		ReplyMarkup: buildPriorityKeyboard(),
	})
}

// handlePriorityCallback handles priority selection for both /support and /ticket flows.
func (h *Handler) handlePriorityCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	priority, err := strconv.Atoi(strings.TrimPrefix(query.Data, "prio:"))
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	pending, ok := h.states[key].(*pendingSession)
	h.mu.Unlock()
	if !ok || pending == nil || pending.Step != StepPriority {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ " + priorityLabel(priority),
	})

	h.mu.Lock()
	pending.Priority = priority
	h.mu.Unlock()

	if pending.Flow == FlowTicket {
		h.createTicketIssue(ctx, b, pending)
	} else {
		h.createSupportIssue(ctx, b, pending)
	}
}

// handleSupportPendingIssue handles multi-step flow for support and ticket requests
func (h *Handler) handleSupportPendingIssue(ctx context.Context, b *tgbot.Bot, msg *models.Message, pending *pendingSession) {
	key := stateKey{UserID: msg.From.ID}
	text := strings.TrimSpace(msg.Text)

	switch pending.Step {
	case StepLinearAccount:
		if text == "" {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      "❌ Linear username cannot be empty. Please enter your Linear username:",
			})
			return
		}

		if err := h.storage.SetUserLinearUsername(ctx, pending.UserID, text); err != nil {
			log.Printf("⚠️  Failed to save linear username for user %d: %v", pending.UserID, err)
		}

		log.Printf("✓ Linear username set for user %d: %s", pending.UserID, text)

		// Standalone update — just confirm and done
		if pending.Flow == FlowUpdateLinear {
			h.mu.Lock()
			delete(h.states, key)
			h.mu.Unlock()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      "✅ Linear account set: " + text,
			})
			return
		}

		categories, err := h.storage.ListCategoriesForContext(ctx, pending.ChatID, pending.ThreadID)
		if err != nil || len(categories) == 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      "❌ No categories configured. Contact admin.",
			})
			h.mu.Lock()
			delete(h.states, key)
			h.mu.Unlock()
			return
		}

		prompt := "🗂️ Select issue category:"
		if pending.Flow == FlowTicket {
			prompt = "🗂️ Select category for this ticket:"
		}

		h.mu.Lock()
		pending.Step = StepCategory
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      pending.ChatID,
			MessageID:   pending.MessageID,
			Text:        "✓ Linear account linked.\n\n" + prompt,
			ReplyMarkup: buildCategoryKeyboard(categories),
		})

	case StepDescription:
		// Collect description text and any attached media
		description := text

		if msg.Photo != nil && len(msg.Photo) > 0 {
			photo := msg.Photo[len(msg.Photo)-1]
			if file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: photo.FileID}); err == nil && file != nil {
				fileLink := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
				description += fmt.Sprintf("\n\n📷 **Photo:**\n[Image](%s)", fileLink)
			}
		}
		if msg.Document != nil {
			if file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: msg.Document.FileID}); err == nil && file != nil {
				fileLink := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
				description += fmt.Sprintf("\n\n📎 **Attachment:**\n[%s](%s)", msg.Document.FileName, fileLink)
			}
		}

		// Load categories before showing the keyboard
		categories, err := h.storage.ListCategoriesForContext(ctx, pending.ChatID, pending.ThreadID)
		if err != nil || len(categories) == 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      "❌ No categories configured. Contact admin.",
			})
			h.mu.Lock()
			delete(h.states, key)
			h.mu.Unlock()
			return
		}

		h.mu.Lock()
		pending.Description = description
		pending.SupportMsgID = msg.ID
		pending.Step = StepCategory
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      pending.ChatID,
			MessageID:   pending.MessageID,
			Text:        "✓ Description saved.\n\n🗂️ Select issue category:",
			ReplyMarkup: buildCategoryKeyboard(categories),
		})
	}
}

// createSupportIssue creates a Linear issue from a standalone /ticket (non-reply) session.
func (h *Handler) createSupportIssue(ctx context.Context, b *tgbot.Bot, pending *pendingSession) {
	title := ticketTitle(pending.Description, pending.CreatedAt)
	if title == "" || strings.TrimSpace(pending.Description) == "" {
		title = fmt.Sprintf("[%s] Ticket from Telegram (%s)", pending.CategoryName, pending.CreatedAt.Format("2006-01-02 15:04"))
	}

	reporter := pending.ReporterName
	if pending.ReporterUsername != "" {
		reporter = fmt.Sprintf("[%s](https://t.me/%s)", pending.ReporterName, pending.ReporterUsername)
	}
	if pending.RequesterLinear != "" {
		reporter += fmt.Sprintf(" / Linear: %s", pending.RequesterLinear)
	}

	tgLink := formatTelegramLink(pending.ChatID, pending.ThreadID, pending.SupportMsgID)
	description := pending.Description + fmt.Sprintf("\n\n---\n\n**📌 Telegram Source**\n"+
		"- **Reporter:** %s\n"+
		"- **Chat:** %s\n"+
		"- **Category:** %s\n"+
		"- **Type:** %s",
		reporter,
		pending.ChatTitle,
		pending.CategoryName,
		pending.TypeName)
	if tgLink != "" {
		description += fmt.Sprintf("\n- **Link:** %s", tgLink)
	}

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

	url, err := h.linear.CreateIssue(ctx, title, description, pending.TeamKey, assignee, []string{pending.CategoryName, pending.TypeName, priorityName(pending.Priority)}, pending.Priority)
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
		onlineIcon := "🟢"
		if !onDutyResult.Online {
			onlineIcon = "🔴"
		}
		assigneeStr = fmt.Sprintf("%s %s\n  🔵 Telegram: @%s\n  🔷 Linear: @%s", person.Name, onlineIcon, person.TelegramUsername, person.LinearUsername)
		if person.Status != "" {
			assigneeStr += "\n  " + statusEmoji(person.Status) + " " + person.Status
		}
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    pending.ChatID,
		MessageID: pending.MessageID,
		Text: fmt.Sprintf(
			"✅ Issue created!\n\n"+
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
	log.Printf("✓ Support issue created: %s (assigned to %s)", url, assignedPersonName)
}

// Helper functions

func parseCategoryID(idStr string) int64 {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	return id
}

func parseTypeID(idStr string) int64 {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	return id
}
