package telegram

import (
	"context"
	"fmt"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log"
	"strconv"
	"strings"
)

func (h *Handler) handleGroups(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendGroupsList(ctx, b, msg.Chat.ID, 0)
}

// sendGroupsList renders the top-level groups list: a header with approved/pending
// counts and one button per group. Selecting a group opens its detail view.
func (h *Handler) sendGroupsList(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int) {
	groups, err := h.storage.ListGroups(ctx)
	if err != nil {
		log.Printf("❌ ListGroups: %v", err)
		return
	}

	if len(groups) == 0 {
		text := h.trans.Admin.NoGroupsRegistered
		if editMsgID != 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{ChatID: chatID, MessageID: editMsgID, Text: text})
		} else {
			b.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatID, Text: text})
		}
		return
	}

	approved, pending := 0, 0
	for _, g := range groups {
		if g.Approved {
			approved++
		} else {
			pending++
		}
	}

	text := fmt.Sprintf(h.trans.Group.GroupsListHeader, approved, pending)

	rows := make([][]models.InlineKeyboardButton, 0, len(groups))
	for _, g := range groups {
		title := g.Title
		if title == "" {
			title = fmt.Sprintf("chat_%d", g.ChatID)
		}
		status := "⏳"
		if g.Approved {
			status = "✅"
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s %s", status, title),
			CallbackData: fmt.Sprintf("grpd:%d", g.ChatID),
		}})
	}

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: rows}

	if editMsgID != 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   editMsgID,
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

// showGroupDetail renders the detail/config view for a single group: its status,
// timezone, and type, plus action buttons (approve/reject, set TZ, toggle type, back).
func (h *Handler) showGroupDetail(ctx context.Context, b *tgbot.Bot, adminChatID int64, msgID int, groupChatID int64) {
	g, err := h.storage.GetGroup(ctx, groupChatID)
	if err != nil {
		log.Printf("❌ GetGroup %d: %v", groupChatID, err)
		return
	}
	if g == nil {
		h.sendGroupsList(ctx, b, adminChatID, msgID)
		return
	}

	title := g.Title
	if title == "" {
		title = fmt.Sprintf("chat_%d", g.ChatID)
	}
	tz := g.Timezone
	if tz == "" {
		tz = "UTC"
	}
	statusStr := h.trans.Group.StatusPending
	if g.Approved {
		statusStr = h.trans.Group.StatusApproved
	}

	text := fmt.Sprintf(h.trans.Group.GroupDetails, title, statusStr, tz, g.ChatID)

	var approveBtn models.InlineKeyboardButton
	if g.Approved {
		approveBtn = models.InlineKeyboardButton{
			Text:         h.trans.Group.BtnReject,
			CallbackData: fmt.Sprintf("disapprove:%d", g.ChatID),
		}
	} else {
		approveBtn = models.InlineKeyboardButton{
			Text:         h.trans.Group.BtnApprove,
			CallbackData: fmt.Sprintf("approve:%d", g.ChatID),
		}
	}

	rows := [][]models.InlineKeyboardButton{
		{approveBtn},
		{
			{Text: h.trans.Group.BtnSetTimezone, CallbackData: fmt.Sprintf("grptz:sel:%d", g.ChatID)},
		},
		{
			{Text: h.trans.Group.BtnBackToList, CallbackData: "grpd:list"},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      adminChatID,
		MessageID:   msgID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// handleGroupDetailCallback handles grpd: callbacks for the groups menu.
// grpd:list      — return to the top-level groups list
// grpd:{chatID}  — open the detail/config view for a group
func (h *Handler) handleGroupDetailCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	data := strings.TrimPrefix(query.Data, "grpd:")
	msg := query.Message.Message

	if data == "list" {
		h.sendGroupsList(ctx, b, msg.Chat.ID, msg.ID)
		return
	}

	chatID, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return
	}
	h.showGroupDetail(ctx, b, msg.Chat.ID, msg.ID, chatID)
}

func (h *Handler) handleGroupApproveCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	approve := strings.HasPrefix(query.Data, "approve:")
	chatIDStr := strings.TrimPrefix(query.Data, "approve:")
	chatIDStr = strings.TrimPrefix(chatIDStr, "disapprove:")

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return
	}

	if err := h.storage.SetGroupApproved(ctx, chatID, approve); err != nil {
		log.Printf("❌ SetGroupApproved: %v", err)
		return
	}

	action := "approved"
	if !approve {
		action = "disapproved"
	}
	log.Printf("✓ Group %d %s by @%s", chatID, action, query.From.Username)

	msg := query.Message.Message
	h.showGroupDetail(ctx, b, msg.Chat.ID, msg.ID, chatID)
}

// handleGroupTZCallback handles grptz: callbacks for group timezone management.
// grptz:sel:{chatID}      — show timezone picker for group
// grptz:set:{chatID}:{tz} — save timezone and return to the group detail view
// grptz:back:{chatID}     — return to the group detail view without saving
func (h *Handler) handleGroupTZCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	data := strings.TrimPrefix(query.Data, "grptz:")
	msg := query.Message.Message

	switch {
	case strings.HasPrefix(data, "sel:"):
		chatIDStr := strings.TrimPrefix(data, "sel:")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			return
		}
		h.showGroupTZPicker(ctx, b, msg.Chat.ID, msg.ID, chatID)

	case strings.HasPrefix(data, "set:"):
		rest := strings.TrimPrefix(data, "set:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return
		}
		chatID, err := strconv.ParseInt(rest[:idx], 10, 64)
		if err != nil {
			return
		}
		tz := rest[idx+1:]
		if _, err := parseLocation(tz); err != nil {
			log.Printf("⚠️ invalid timezone %q: %v", tz, err)
			return
		}
		if err := h.storage.SetGroupTimezone(ctx, chatID, tz); err != nil {
			log.Printf("❌ SetGroupTimezone %d %q: %v", chatID, tz, err)
			return
		}
		log.Printf("✓ Group %d timezone set to %q by @%s", chatID, tz, query.From.Username)
		h.showGroupDetail(ctx, b, msg.Chat.ID, msg.ID, chatID)

	case strings.HasPrefix(data, "back:"):
		chatIDStr := strings.TrimPrefix(data, "back:")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			return
		}
		h.showGroupDetail(ctx, b, msg.Chat.ID, msg.ID, chatID)
	}
}

func (h *Handler) showGroupTZPicker(ctx context.Context, b *tgbot.Bot, adminChatID int64, msgID int, groupChatID int64) {
	tzs, _, _, err := h.storage.GetSupportPersonDefaults(ctx)
	if err != nil {
		log.Printf("❌ GetSupportPersonDefaults: %v", err)
	}

	// Merge with common presets so there's always something to pick even on a fresh install
	presets := []string{"UTC", "+05:00", "+06:00", "+03:00", "+04:00"}
	seen := map[string]bool{}
	for _, t := range tzs {
		seen[t] = true
	}
	for _, p := range presets {
		if !seen[p] {
			tzs = append(tzs, p)
			seen[p] = true
		}
	}

	prefix := fmt.Sprintf("grptz:set:%d:", groupChatID)
	kb := buildPickerKeyboard(tzs, prefix, false)

	// Add a Back button at the bottom
	kb.InlineKeyboard = append(kb.InlineKeyboard, []models.InlineKeyboardButton{{
		Text:         h.trans.Admin.Back,
		CallbackData: fmt.Sprintf("grptz:back:%d", groupChatID),
	}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      adminChatID,
		MessageID:   msgID,
		Text:        h.trans.Person.EnterTimezone,
		ReplyMarkup: kb,
	})
}
