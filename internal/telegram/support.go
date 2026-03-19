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

// handleSupportStart initiates the /support flow
func (h *Handler) handleSupportStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	log.Printf("✓ Processing /support command from chat_id: %d\n", msg.Chat.ID)

	// Load categories from DB (topic-aware: show global + topic-specific categories)
	categories, err := h.storage.ListCategoriesForContext(ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}

	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No categories configured. Contact admin.")
		return
	}

	// Build inline keyboard
	keyboard := buildCategoryKeyboard(categories)

	params := &tgbot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        "🗂️ Select issue category:",
		ReplyMarkup: keyboard,
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}

	sentMsg, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	reporterName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)

	// Store session state
	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingSession{
		Flow:             FlowSupport,
		Step:             StepCategory,
		UserID:           msg.From.ID,
		MessageID:        sentMsg.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		CreatedAt:        time.Now(),
		ReporterName:     reporterName,
		ReporterUsername: msg.From.Username,
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
	if !ok {
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
	if !ok {
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
		h.mu.Lock()
		pending.Step = StepTitle
		h.mu.Unlock()

		// Collapse the priority keyboard — removes inline buttons from the selection message
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      "✓ " + priorityLabel(priority),
		})

		// Send a separate ForceReply message so the title text reaches the bot even
		// when group privacy mode is on (only replies to bot messages are delivered).
		sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:          pending.ChatID,
			MessageThreadID: pending.ThreadID,
			Text:            "📝 Enter issue title:",
			ReplyMarkup:     &models.ForceReply{ForceReply: true},
		})
		if err != nil {
			log.Printf("⚠️  StepTitle SendMessage failed (chat=%d): %v", pending.ChatID, err)
			return
		}
		h.mu.Lock()
		pending.MessageID = sentMsg.ID
		h.mu.Unlock()
	}
}

// handleSupportPendingIssue handles multi-step flow for support requests
func (h *Handler) handleSupportPendingIssue(ctx context.Context, b *tgbot.Bot, msg *models.Message, pending *pendingSession) {
	key := stateKey{UserID: msg.From.ID}
	text := strings.TrimSpace(msg.Text)

	switch pending.Step {
	case StepTitle:
		if text == "" {
			sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID:          pending.ChatID,
				MessageThreadID: pending.ThreadID,
				Text:            "❌ Title cannot be empty. Enter issue title:",
				ReplyMarkup:     &models.ForceReply{ForceReply: true},
			})
			if err == nil {
				h.mu.Lock()
				pending.MessageID = sentMsg.ID
				h.mu.Unlock()
			}
			return
		}

		h.mu.Lock()
		pending.Title = text
		pending.Step = StepDescription
		h.mu.Unlock()

		// Collapse the title prompt to a summary
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("✓ Title: %s", text),
		})

		// Send a separate ForceReply for description so it reaches the bot with privacy mode on
		sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:          pending.ChatID,
			MessageThreadID: pending.ThreadID,
			Text:            "📝 Enter description (optional, or send media):",
			ReplyMarkup:     &models.ForceReply{ForceReply: true},
		})
		if err != nil {
			log.Printf("⚠️  StepDescription SendMessage failed (chat=%d): %v", pending.ChatID, err)
		} else {
			h.mu.Lock()
			pending.MessageID = sentMsg.ID
			h.mu.Unlock()
		}

	case StepDescription:
		// DON'T delete user's message - preserve data
		// User's original message will remain in Telegram for reference

		// Collect description and handle media
		h.mu.Lock()
		title := pending.Title
		teamKey := pending.TeamKey
		categoryName := pending.CategoryName
		typeName := pending.TypeName
		delete(h.states, key)
		h.mu.Unlock()

		description := text // Empty string is OK

		// Handle photos/documents
		if msg.Photo != nil && len(msg.Photo) > 0 {
			// Get highest resolution photo
			photo := msg.Photo[len(msg.Photo)-1]
			// Try to get file info and build download link
			file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: photo.FileID})
			if err == nil && file != nil {
				fileLink := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
				description += fmt.Sprintf("\n\n📷 **Photo:**\n[Image](%s)", fileLink)
			}
		}

		if msg.Document != nil {
			file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: msg.Document.FileID})
			if err == nil && file != nil {
				fileLink := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
				description += fmt.Sprintf("\n\n📎 **Attachment:**\n[%s](%s)", msg.Document.FileName, fileLink)
			}
		}

		// Add Telegram context
		tgLink := formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, msg.ID)
		reporter := pending.ReporterName
		if pending.ReporterUsername != "" {
			reporter = fmt.Sprintf("%s (@%s)", pending.ReporterName, pending.ReporterUsername)
		}
		description += fmt.Sprintf("\n\n---\n\n**📌 Telegram Source**\n"+
			"- **Reporter:** %s\n"+
			"- **Chat:** %s\n"+
			"- **Category:** %s\n"+
			"- **Type:** %s",
			reporter,
			msg.Chat.Title,
			categoryName,
			typeName)
		if tgLink != "" {
			description += fmt.Sprintf("\n- **Link:** %s", tgLink)
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

		// Add offline warning to description if person is outside working hours
		if onDutyResult != nil && !onDutyResult.Online {
			description += "\n\n⚠️ **Note:** Assigned person is currently outside working hours."
		}

		// Create Linear issue with category and type as labels
		url, err := h.linear.CreateIssue(ctx, title, description, teamKey, assignee, []string{pending.CategoryName, pending.TypeName}, pending.Priority)
		if err != nil {
			log.Printf("❌ Failed to create Linear issue: %v\n", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    pending.ChatID,
				MessageID: pending.MessageID,
				Text:      fmt.Sprintf("❌ Failed to create issue: %v", err),
			})
			return
		}

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
			"✅ Issue created!\n\n"+
				"📝 Title: %s\n"+
				"📄 Description: %s\n"+
				"👤 Assigned to: %s\n"+
				"🔗 Link: %s",
			title,
			truncateStr(text, 50),
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
		log.Printf("✓ Issue created: %s (assigned to %s)", url, assignedPersonName)
	}
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

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
