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

// handleOffboard starts the /offboard admin flow.
func (h *Handler) handleOffboard(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	key := stateKey{UserID: msg.From.ID}
	admin := &pendingAdminSession{
		Cmd:       AdminCmdOffboard,
		Step:      StepOffboardUsername,
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		CreatedAt: time.Now(),
	}

	sent, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Enter the Telegram username to offboard (with or without @):",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Cancel", CallbackData: "cancel"}},
			},
		},
	})
	if err != nil {
		log.Printf("offboard: send prompt error: %v", err)
		return
	}

	admin.MessageID = sent.ID
	h.mu.Lock()
	h.states[key] = admin
	h.mu.Unlock()
}

// handleOffboardPending handles the username input step.
func (h *Handler) handleOffboardPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	if admin.Step != StepOffboardUsername {
		return
	}

	username := strings.TrimSpace(strings.TrimPrefix(msg.Text, "@"))
	if username == "" {
		return
	}

	key := stateKey{UserID: msg.From.ID}

	userID, err := h.storage.LookupUserByUsername(ctx, username)
	if err != nil {
		log.Printf("offboard: lookup error: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      "Error looking up user. Please try again.",
		})
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		return
	}
	if userID == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("User @%s not found in bot database. They may not have interacted with the bot.", username),
		})
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		return
	}

	groupIDs, err := h.storage.ListApprovedGroupIDs(ctx)
	if err != nil {
		log.Printf("offboard: list groups error: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      "Error listing groups. Please try again.",
		})
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		return
	}

	type entry struct {
		chatID int64
		title  string
	}
	var found []entry
	for _, chatID := range groupIDs {
		member, err := b.GetChatMember(ctx, &tgbot.GetChatMemberParams{
			ChatID: chatID,
			UserID: userID,
		})
		if err != nil {
			continue
		}
		switch member.Type {
		case models.ChatMemberTypeOwner,
			models.ChatMemberTypeAdministrator,
			models.ChatMemberTypeMember,
			models.ChatMemberTypeRestricted:
			found = append(found, entry{chatID: chatID, title: h.offboardGroupTitle(chatID)})
		}
	}

	if len(found) == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("@%s is not a member of any bot-managed groups.", username),
		})
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		return
	}

	admin.OffboardUserID = userID
	admin.OffboardUsername = username
	admin.OffboardGroupIDs = make([]int64, len(found))
	for i, g := range found {
		admin.OffboardGroupIDs[i] = g.chatID
	}
	admin.Step = StepOffboardConfirm
	h.mu.Lock()
	h.states[key] = admin
	h.mu.Unlock()

	var rows [][]models.InlineKeyboardButton
	for _, g := range found {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         g.title,
			CallbackData: fmt.Sprintf("offbrd_grp:%d:%d", g.chatID, userID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         "Remove from all groups",
		CallbackData: fmt.Sprintf("offbrd_all:%d", userID),
	}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "cancel"}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        fmt.Sprintf("@%s is in %d group(s). Select a group to remove them from:", username, len(found)),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// handleOffboardGroupCallback removes the user from one specific group.
func (h *Handler) handleOffboardGroupCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	// offbrd_grp:{chatID}:{userID}
	parts := strings.SplitN(strings.TrimPrefix(query.Data, "offbrd_grp:"), ":", 2)
	if len(parts) != 2 {
		return
	}
	targetChatID, err1 := strconv.ParseInt(parts[0], 10, 64)
	targetUserID, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	raw, ok := h.states[key]
	h.mu.Unlock()
	if !ok {
		return
	}
	admin, ok := raw.(*pendingAdminSession)
	if !ok || admin.Cmd != AdminCmdOffboard || admin.Step != StepOffboardConfirm {
		return
	}

	_, err := b.BanChatMember(ctx, &tgbot.BanChatMemberParams{
		ChatID: targetChatID,
		UserID: targetUserID,
	})
	if err != nil {
		log.Printf("offboard: ban in %d error: %v", targetChatID, err)
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Failed to remove user. Check bot permissions.",
			ShowAlert:       true,
		})
		return
	}

	groupName := h.offboardGroupTitle(targetChatID)

	var remaining []int64
	for _, id := range admin.OffboardGroupIDs {
		if id != targetChatID {
			remaining = append(remaining, id)
		}
	}
	admin.OffboardGroupIDs = remaining

	if len(remaining) == 0 {
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("@%s has been removed from all groups.", admin.OffboardUsername),
		})
		return
	}

	h.mu.Lock()
	h.states[key] = admin
	h.mu.Unlock()

	var rows [][]models.InlineKeyboardButton
	for _, id := range remaining {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         h.offboardGroupTitle(id),
			CallbackData: fmt.Sprintf("offbrd_grp:%d:%d", id, targetUserID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         "Remove from all groups",
		CallbackData: fmt.Sprintf("offbrd_all:%d", targetUserID),
	}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "cancel"}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        fmt.Sprintf("Removed @%s from %s. %d group(s) remaining:", admin.OffboardUsername, groupName, len(remaining)),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// handleOffboardAllCallback removes the user from all remaining groups.
func (h *Handler) handleOffboardAllCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	// offbrd_all:{userID}
	targetUserID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "offbrd_all:"), 10, 64)
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	raw, ok := h.states[key]
	h.mu.Unlock()
	if !ok {
		return
	}
	admin, ok := raw.(*pendingAdminSession)
	if !ok || admin.Cmd != AdminCmdOffboard || admin.Step != StepOffboardConfirm {
		return
	}

	total := len(admin.OffboardGroupIDs)
	var failed []string
	for _, chatID := range admin.OffboardGroupIDs {
		_, err := b.BanChatMember(ctx, &tgbot.BanChatMemberParams{
			ChatID: chatID,
			UserID: targetUserID,
		})
		if err != nil {
			log.Printf("offboard: ban in %d error: %v", chatID, err)
			failed = append(failed, h.offboardGroupTitle(chatID))
		}
	}

	h.mu.Lock()
	delete(h.states, key)
	h.mu.Unlock()

	var text string
	if len(failed) == 0 {
		text = fmt.Sprintf("@%s has been removed from all %d group(s).", admin.OffboardUsername, total)
	} else {
		text = fmt.Sprintf("@%s removed from %d/%d groups. Failed: %s",
			admin.OffboardUsername, total-len(failed), total, strings.Join(failed, ", "))
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    admin.ChatID,
		MessageID: admin.MessageID,
		Text:      text,
	})
}

// offboardGroupTitle returns the display name for a group, falling back to the chat ID.
func (h *Handler) offboardGroupTitle(chatID int64) string {
	h.mu.Lock()
	title := h.groups[chatID]
	h.mu.Unlock()
	if title != "" {
		return title
	}
	return fmt.Sprintf("Group %d", chatID)
}
