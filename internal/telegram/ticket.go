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

// handleTicketStart initiates the /ticket flow. If msg is a reply, the replied-to
// message is used as the ticket source. Otherwise it falls back to the support flow.
func (h *Handler) handleTicketStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if msg.ReplyToMessage == nil {
		h.handleSupportStart(ctx, b, msg)
		return
	}

	replied := msg.ReplyToMessage

	link := formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, replied.ID)

	// For media messages the text lives in Caption, not Text
	body := replied.Text
	if body == "" {
		body = replied.Caption
	}

	// Extract media links from the replied message
	var mediaLinks []string
	if len(replied.Photo) > 0 {
		photo := replied.Photo[len(replied.Photo)-1]
		if file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: photo.FileID}); err == nil && file != nil {
			mediaLinks = append(mediaLinks, fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath))
		}
	}
	if replied.Document != nil {
		if file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: replied.Document.FileID}); err == nil && file != nil {
			mediaLinks = append(mediaLinks, fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath))
		}
	}

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
		if msg.Chat.Type == "private" {
			h.sendMessage(ctx, b, msg, "⚠️ /ticket must be used in a group chat, not here in DM.")
		} else {
			h.sendMessage(ctx, b, msg, h.buildUnconfiguredTopicMsg(ctx, msg.Chat.ID))
		}
		return
	}

	linearUsername, _ := h.storage.GetUserLinearUsername(ctx, msg.From.ID)

	text := "🗂️ Select category for this ticket:"
	if linearUsername == "" {
		text = "⚠️ Your Telegram account is not linked to Linear. Use /mylinear to link it.\n\n" + text
	}

	session := &pendingSession{
		Flow:             FlowTicket,
		Step:             StepCategory,
		UserID:           msg.From.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		CreatedAt:        time.Now(),
		TicketMsgLink:    link,
		TicketMsgBody:    body,
		TicketMsgDate:    time.Unix(int64(replied.Date), 0),
		MediaLinks:       mediaLinks,
		ReporterName:     reporterName,
		ReporterUsername: reporterUsername,
		RequesterLinear:  linearUsername,
	}

	var sentMsg *models.Message
	sentMsg, err = b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            text,
		ReplyMarkup:     buildCategoryKeyboard(categories),
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v\n", err)
		return
	}

	session.MessageID = sentMsg.ID
	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = session
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

// buildUnconfiguredTopicMsg returns a message for when /ticket is used in a group topic with no categories.
// It lists other topics in the same group that do have categories configured.
func (h *Handler) buildUnconfiguredTopicMsg(ctx context.Context, chatID int64) string {
	topics, err := h.storage.ListConfiguredTopicsForChat(ctx, chatID)
	if err != nil || len(topics) == 0 {
		return "⚠️ This topic is not configured for support tickets yet. Contact an admin."
	}
	msg := "⚠️ This topic is not configured for support tickets yet.\n\nYou can use /ticket in:"
	for _, t := range topics {
		msg += "\n• " + t
	}
	return msg
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
		reporter = fmt.Sprintf("[%s](https://t.me/%s)", pending.ReporterName, pending.ReporterUsername)
	}
	if pending.RequesterLinear != "" {
		reporter += fmt.Sprintf(" / Linear: %s", pending.RequesterLinear)
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

	if pending.TicketMsgBody != "" || len(pending.MediaLinks) > 0 {
		var msgSection string
		if pending.TicketMsgBody != "" {
			msgSection = fmt.Sprintf("**💬 Message**\n%s", pending.TicketMsgBody)
		} else {
			msgSection = "**💬 Message**"
		}
		if len(pending.MediaLinks) > 0 {
			msgSection += formatMediaLinks(pending.MediaLinks)
		}
		description = msgSection + "\n\n" + description
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
