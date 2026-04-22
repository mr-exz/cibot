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

// handleSetLabelForward is triggered when an admin forwards a user message to the bot in DM.
// It starts the setlabel flow: capture user ID → ask for label → pick group → call setChatMemberTag.
func (h *Handler) handleSetLabelForward(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	origin := msg.ForwardOrigin.MessageOriginUser.SenderUser
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("👤 User: %s (@%s)\n\n✏️ Enter the label to set:", strings.TrimSpace(origin.FirstName+" "+origin.LastName), origin.Username),
	})
	if err != nil {
		log.Printf("❌ handleSetLabelForward send: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:           AdminCmdSetLabel,
		Step:          StepAdminSetLabelWaitLabel,
		MessageID:     sentMsg.ID,
		ChatID:        msg.Chat.ID,
		CreatedAt:     time.Now(),
		UserID:        msg.From.ID,
		LabelUserID:   origin.ID,
		LabelUsername: origin.Username,
	}
	h.mu.Unlock()
	log.Printf("✓ setlabel flow started for user %d (@%s) by %s", origin.ID, origin.Username, msg.From.Username)
}

// handleAddCategory starts the /addcategory interactive flow
func (h *Handler) handleAddCategory(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	log.Printf("📝 /addcategory from %s (chat_id: %d)", msg.From.Username, msg.Chat.ID)

	dbGroups, err := h.storage.ListGroups(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load groups: %v", err))
		return
	}

	groups := make(map[int64]string)
	for _, g := range dbGroups {
		if g.Approved {
			groups[g.ChatID] = g.Title
		}
	}

	if len(groups) == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ No approved groups registered yet.")
		return
	}

	keyboard := buildGroupKeyboard(groups)
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{
		Text: "❌ Cancel", CallbackData: "cancel",
	}})

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🏘️ Select group:",
		MessageThreadID: msg.MessageThreadID,
		ReplyMarkup:     keyboard,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdAddCategory,
		Step:      StepAdminCatSelectGroup,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()

	log.Printf("✓ Started /addcategory flow for %s", msg.From.Username)
}

// handleAddType starts the /addtype interactive flow
func (h *Handler) handleAddType(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.startAdminCategoryPicker(ctx, b, msg, AdminCmdAddType)
}

// handleAddPerson starts the /addperson interactive flow
func (h *Handler) handleAddPerson(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.startAdminCategoryPicker(ctx, b, msg, AdminCmdAddPerson)
}

// handleSetRotation starts the /setrotation interactive flow
func (h *Handler) handleSetRotation(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.startAdminCategoryPicker(ctx, b, msg, AdminCmdSetRotation)
}

// handleSetWorkHours starts the /setworkhours interactive flow
func (h *Handler) handleSetWorkHours(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Get all support persons to show selection
	persons, err := h.storage.ListAllSupportPersons(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load support persons: %v", err))
		return
	}

	if len(persons) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No support persons available. Add one first with /addperson")
		return
	}

	keyboard := buildPersonKeyboard(persons)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "👤 Select person:",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdSetWorkHours,
		Step:      StepAdminSelectPerson,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /setworkhours flow for %s", msg.From.Username)
}

// handleAddTopic starts the /addtopic interactive flow
func (h *Handler) handleAddTopic(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	allGroups, err := h.storage.ListGroups(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load groups: %v", err))
		return
	}
	groups := make(map[int64]string)
	for _, g := range allGroups {
		if g.Approved {
			title := g.Title
			if title == "" {
				title = fmt.Sprintf("Group %d", g.ChatID)
			}
			groups[g.ChatID] = title
		}
	}
	if len(groups) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No approved groups yet. Approve groups via /groups first.")
		return
	}

	keyboard := buildGroupKeyboard(groups)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🏘️ Select group:",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdAddTopic,
		Step:      StepAdminTopicSelectGroup,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /addtopic flow for %s", msg.From.Username)
}

// handleListTopics shows all registered topics across all known groups
func (h *Handler) handleListTopics(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	allTopics := h.getAllTopics()

	if len(allTopics) == 0 {
		h.sendMessage(ctx, b, msg, "📭 No topics registered yet. Use /addtopic to register forum topics.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Registered Topics:\n")

	for chatID, topics := range allTopics {
		groupName := h.getGroupName(chatID)
		sb.WriteString(fmt.Sprintf("\n%s (chat_id: %d):\n", groupName, chatID))
		for threadID, topicName := range topics {
			sb.WriteString(fmt.Sprintf("  🔹 thread %d — %s\n", threadID, topicName))
		}
	}

	sb.WriteString("\nUse /addcategory to link categories to topics.")
	h.sendMessage(ctx, b, msg, sb.String())
}

const usersPageSize = 20

// handleUsers lists all known users from telegram_user_metadata with Set Tag buttons.
func (h *Handler) handleUsers(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendUsersPage(ctx, b, msg.Chat.ID, 0, 0)
}

func (h *Handler) sendUsersPage(ctx context.Context, b *tgbot.Bot, chatID int64, existingMsgID int, offset int) {
	total, err := h.storage.CountUsers(ctx)
	if err != nil {
		b.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatID, Text: fmt.Sprintf("❌ %v", err)})
		return
	}
	if total == 0 {
		b.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatID, Text: "📭 No users seen yet."})
		return
	}

	users, err := h.storage.ListUsers(ctx, usersPageSize, offset)
	if err != nil {
		b.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatID, Text: fmt.Sprintf("❌ %v", err)})
		return
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(users)+1)
	for _, u := range users {
		label := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if u.Username != "" {
			label += " (@" + u.Username + ")"
		}
		if u.LinearUsername != "" {
			label += " 🔷"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{
				Text:         "👤 " + label,
				CallbackData: fmt.Sprintf("usr:%d:%d", u.UserID, offset),
			},
		})
	}

	// Pagination row
	var navRow []models.InlineKeyboardButton
	if offset > 0 {
		navRow = append(navRow, models.InlineKeyboardButton{
			Text:         "◀ Prev",
			CallbackData: fmt.Sprintf("usrp:%d", offset-usersPageSize),
		})
	}
	if offset+usersPageSize < total {
		navRow = append(navRow, models.InlineKeyboardButton{
			Text:         "▶ Next",
			CallbackData: fmt.Sprintf("usrp:%d", offset+usersPageSize),
		})
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	text := fmt.Sprintf("👥 Known users (%d–%d of %d):", offset+1, min(offset+len(users), total), total)
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: rows}

	if existingMsgID != 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   existingMsgID,
			Text:        text,
			ReplyMarkup: keyboard,
		})
	} else {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        text,
			ReplyMarkup: keyboard,
		})
	}
}

// handleUserPageCallback handles ◀/▶ pagination in /users list.
func (h *Handler) handleUserPageCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	offset, err := strconv.Atoi(strings.TrimPrefix(query.Data, "usrp:"))
	if err != nil {
		return
	}
	msg := query.Message.Message
	if msg == nil {
		return
	}
	h.sendUsersPage(ctx, b, msg.Chat.ID, msg.ID, offset)
}

// handleUserDetailCallback handles tapping a user in /users — shows a detail view.
func (h *Handler) handleUserDetailCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	// data format: usr:{userID}:{offset}
	parts := strings.SplitN(strings.TrimPrefix(query.Data, "usr:"), ":", 2)
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}
	offset := 0
	if len(parts) == 2 {
		offset, _ = strconv.Atoi(parts[1])
	}

	targetUser, err := h.storage.GetUserByID(ctx, userID)
	if err != nil || targetUser == nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "User not found"})
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	name := strings.TrimSpace(targetUser.FirstName + " " + targetUser.LastName)
	text := fmt.Sprintf("👤 %s", name)
	if targetUser.Username != "" {
		text += fmt.Sprintf("\n🔵 Telegram: @%s", targetUser.Username)
	}
	if targetUser.LinearUsername != "" {
		text += fmt.Sprintf("\n🔷 Linear: @%s", targetUser.LinearUsername)
	} else {
		text += "\n🔷 Linear: not linked"
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🏷 Set Tag", CallbackData: fmt.Sprintf("usrst:%d", targetUser.UserID)},
				{Text: "🗑 Delete", CallbackData: fmt.Sprintf("usrc:%d", targetUser.UserID)},
			},
			{
				{Text: "◀ Back", CallbackData: fmt.Sprintf("usrp:%d", offset)},
			},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
}

// handleUserSetTagCallback handles Set Tag button in user detail — starts the setlabel flow.
func (h *Handler) handleUserSetTagCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	userID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "usrst:"), 10, 64)
	if err != nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	targetUser, err := h.storage.GetUserByID(ctx, userID)
	if err != nil || targetUser == nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "User not found"})
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	displayName := strings.TrimSpace(targetUser.FirstName + " " + targetUser.LastName)
	if targetUser.Username != "" {
		displayName += " (@" + targetUser.Username + ")"
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("👤 %s\n\n✏️ Enter the label to set:", displayName),
	})
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:           AdminCmdSetLabel,
		Step:          StepAdminSetLabelWaitLabel,
		MessageID:     sentMsg.ID,
		ChatID:        msg.Chat.ID,
		CreatedAt:     time.Now(),
		UserID:        query.From.ID,
		LabelUserID:   targetUser.UserID,
		LabelUsername: targetUser.Username,
	}
	h.mu.Unlock()
	log.Printf("✓ setlabel flow started for user %d (@%s) via /users by %s", targetUser.UserID, targetUser.Username, query.From.Username)
}

// handleUserClearCallback handles 🗑 — starts the clear-tag flow (skips label input, empty tag).
func (h *Handler) handleUserClearCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	userID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "usrc:"), 10, 64)
	if err != nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	targetUser, err := h.storage.GetUserByID(ctx, userID)
	if err != nil || targetUser == nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "User not found"})
		return
	}

	allGroups, err := h.storage.ListGroups(ctx)
	if err != nil {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Failed to load groups"})
		return
	}
	approvedGroups := make(map[int64]string)
	for _, g := range allGroups {
		if g.Approved {
			approvedGroups[g.ChatID] = g.Title
		}
	}
	if len(approvedGroups) == 0 {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "No approved groups"})
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	displayName := strings.TrimSpace(targetUser.FirstName + " " + targetUser.LastName)
	if targetUser.Username != "" {
		displayName += " (@" + targetUser.Username + ")"
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        fmt.Sprintf("🗑 Clear tag for %s\n\n🏘 Select the group:", displayName),
		ReplyMarkup: buildGroupKeyboard(approvedGroups),
	})
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:           AdminCmdSetLabel,
		Step:          StepAdminSetLabelGroup,
		MessageID:     sentMsg.ID,
		ChatID:        msg.Chat.ID,
		CreatedAt:     time.Now(),
		UserID:        query.From.ID,
		LabelUserID:   targetUser.UserID,
		LabelUsername: targetUser.Username,
		LabelText:     "", // empty = clear
	}
	h.mu.Unlock()
	log.Printf("✓ cleartag flow started for user %d (@%s) by %s", targetUser.UserID, targetUser.Username, query.From.Username)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleRotation shows current on-duty support persons for all categories
func (h *Handler) handleRotation(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	duties, err := h.storage.ListAllOnDuty(ctx, time.Now())
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to get rotation: %v", err))
		return
	}

	if len(duties) == 0 {
		h.sendMessage(ctx, b, msg, "No categories with assigned support persons")
		return
	}

	var response string
	response = "📋 Current On-Duty Support\n\n"

	for _, duty := range duties {
		status := "🟢"
		if !duty.Online {
			status = "🔴"
		}
		response += fmt.Sprintf("%s %s → %s %s\n  🔵 @%s | 🔷 @%s\n\n",
			duty.Category.Emoji,
			duty.Category.Name,
			duty.Person.Name,
			status,
			duty.Person.TelegramUsername,
			duty.Person.LinearUsername,
		)
	}

	h.sendMessage(ctx, b, msg, response)
}
