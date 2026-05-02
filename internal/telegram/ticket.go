package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// handleTicketStart initiates the /ticket flow. msg must be a reply to an existing
// message — that message is used as the ticket source. Returns an error otherwise.
func (h *Handler) handleTicketStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if msg.ReplyToMessage == nil {
		h.sendMessage(ctx, b, msg, "⚠️ /ticket requires a reply. Reply to a message to create a ticket from it, or use /ticket_manual to describe the issue yourself.")
		return
	}

	// In forum/topic groups every message implicitly "replies" to the topic header
	// (the service message that created the topic), whose ID equals the thread ID.
	// This is not a real user reply — show an error.
	if msg.MessageThreadID != 0 && msg.ReplyToMessage.ID == msg.MessageThreadID {
		h.sendMessage(ctx, b, msg, "⚠️ /ticket requires a reply to a user message. Use /ticket_manual to describe the issue yourself.")
		return
	}

	replied := msg.ReplyToMessage

	link := formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, replied.ID)

	// For media messages the text lives in Caption, not Text
	body := replied.Text
	if body == "" {
		body = replied.Caption
	}

	// Extract media metadata from the replied message (downloaded and uploaded to Linear later)
	var ticketMedia []*threadMedia
	if meta := extractThreadMedia(replied); meta != nil {
		ticketMedia = append(ticketMedia, meta)
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
		TicketMedia:      ticketMedia,
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

// uploadTicketMedia downloads each media file from Telegram, uploads it to Linear, and returns
// a markdown snippet with inline image previews (or plain links for non-images).
func uploadTicketMedia(ctx context.Context, b *tgbot.Bot, h *Handler, media []*threadMedia) string {
	if len(media) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, meta := range media {
		if meta.fileSize > maxThreadMediaSize {
			log.Printf("⚠️  ticket media: %s too large (%d bytes), skipping", meta.filename, meta.fileSize)
			continue
		}
		file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: meta.fileID})
		if err != nil {
			log.Printf("⚠️  ticket media: GetFile failed: %v", err)
			continue
		}
		dlURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
		if err != nil {
			log.Printf("⚠️  ticket media: request error: %v", err)
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("⚠️  ticket media: download failed: %v", err)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("⚠️  ticket media: read failed: %v", err)
			continue
		}
		assetURL, err := h.linear.UploadFile(ctx, meta.filename, meta.contentType, data)
		if err != nil {
			log.Printf("⚠️  ticket media: Linear upload failed: %v", err)
			continue
		}
		if strings.HasPrefix(meta.contentType, "image/") {
			sb.WriteString(fmt.Sprintf("\n\n![%s](%s)", meta.filename, assetURL))
		} else {
			sb.WriteString(fmt.Sprintf("\n\n[%s](%s)", meta.filename, assetURL))
		}
	}
	return sb.String()
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

	if pending.TicketMsgBody != "" || len(pending.TicketMedia) > 0 {
		var msgSection string
		if pending.TicketMsgBody != "" {
			msgSection = fmt.Sprintf("**💬 Message**\n%s", pending.TicketMsgBody)
		} else {
			msgSection = "**💬 Message**"
		}
		if len(pending.TicketMedia) > 0 {
			msgSection += uploadTicketMedia(ctx, b, h, pending.TicketMedia)
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

	issue, err := h.linear.CreateIssue(ctx, title, description, pending.TeamKey, assignee, []string{pending.CategoryName, pending.TypeName, priorityName(pending.Priority)}, pending.Priority)
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
				"👤 Assigned to: %s",
			pending.CategoryName,
			assigneeStr,
		),
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Linear: " + issue.Identifier, URL: issue.URL}},
			},
		},
	})

	assignedPersonName := "unassigned"
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignedPersonName = onDutyResult.Person.Name
	}
	log.Printf("✓ Ticket created: %s (assigned to %v)", issue.URL, assignedPersonName)
}
