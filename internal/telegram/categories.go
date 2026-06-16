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

func (h *Handler) handleManageCategories(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendCategoryList(ctx, b, msg.Chat.ID, 0)
}

func (h *Handler) sendCategoryList(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int) {
	cats, err := h.storage.ListCategories(ctx)
	if err != nil {
		log.Printf("❌ ListCategories: %v", err)
		return
	}

	text := fmt.Sprintf(h.trans.Admin.Categories, len(cats))
	if len(cats) == 0 {
		text = h.trans.Category.NoCategories
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(cats))
	for _, cat := range cats {
		scope := h.categoryScope(cat)
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s %s · %s", cat.Emoji, cat.Name, scope),
			CallbackData: fmt.Sprintf("catmgr:detail:%d", cat.ID),
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

func (h *Handler) sendCategoryDetail(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, cat *storage.Category) {
	scope := h.categoryScope(*cat)
	text := fmt.Sprintf("📂 %s %s\n🔷 Team: %s\n🌍 Scope: %s", cat.Emoji, cat.Name, cat.LinearTeamKey, scope)

	rows := [][]models.InlineKeyboardButton{
		{
			{Text: h.trans.Category.EditName, CallbackData: fmt.Sprintf("catmgr:editname:%d", cat.ID)},
			{Text: h.trans.Category.EditEmoji, CallbackData: fmt.Sprintf("catmgr:editemoji:%d", cat.ID)},
			{Text: h.trans.Category.EditKey, CallbackData: fmt.Sprintf("catmgr:editkey:%d", cat.ID)},
		},
		{{Text: h.trans.Category.MakeGlobal, CallbackData: fmt.Sprintf("catmgr:global:%d", cat.ID)}},
		{{Text: h.trans.Category.GroupLevel, CallbackData: fmt.Sprintf("catmgr:group:%d", cat.ID)}},
		{{Text: fmt.Sprintf(h.trans.Category.TopicLevel, "📌 Topic"), CallbackData: fmt.Sprintf("catmgr:topic:%d", cat.ID)}},
		{{Text: h.trans.Category.CloneCategory, CallbackData: fmt.Sprintf("catmgr:clone:%d", cat.ID)}},
		{{Text: h.trans.Admin.Confirm, CallbackData: fmt.Sprintf("catmgr:delete:%d", cat.ID)}},
		{{Text: h.trans.Admin.Back, CallbackData: "catmgr:list"}},
	}
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   editMsgID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (h *Handler) categoryScope(cat storage.Category) string {
	if cat.ChatID == nil {
		return "global"
	}
	groupName := h.getGroupName(*cat.ChatID)
	if cat.ThreadID == nil || *cat.ThreadID == 0 {
		return "group: " + groupName
	}
	topics := h.getTopics(*cat.ChatID)
	if topicName, ok := topics[*cat.ThreadID]; ok {
		return "topic: " + topicName
	}
	return fmt.Sprintf("topic #%d", *cat.ThreadID)
}

func (h *Handler) handleCategoryManagerCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	msg := query.Message.Message
	if msg == nil {
		return
	}

	action := strings.TrimPrefix(query.Data, "catmgr:")

	// catmgr:list
	if action == "list" {
		h.sendCategoryList(ctx, b, msg.Chat.ID, msg.ID)
		return
	}

	// catmgr:detail:{catID}
	if strings.HasPrefix(action, "detail:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "detail:"), 10, 64)
		if err != nil {
			return
		}
		cat, err := h.storage.GetCategory(ctx, catID)
		if err != nil {
			return
		}
		h.sendCategoryDetail(ctx, b, msg.Chat.ID, msg.ID, cat)
		return
	}

	// catmgr:global:{catID}
	if strings.HasPrefix(action, "global:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "global:"), 10, 64)
		if err != nil {
			return
		}
		if err := h.storage.UpdateCategoryScope(ctx, catID, nil, nil); err != nil {
			log.Printf("❌ UpdateCategoryScope: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      fmt.Sprintf(h.trans.Error.Failed, err),
			})
			return
		}
		cat, _ := h.storage.GetCategory(ctx, catID)
		if cat != nil {
			h.sendCategoryDetail(ctx, b, msg.Chat.ID, msg.ID, cat)
		}
		return
	}

	// catmgr:group:{catID} — pick a group to set group-level scope
	if strings.HasPrefix(action, "group:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "group:"), 10, 64)
		if err != nil {
			return
		}
		h.showGroupPickerForCatMgr(ctx, b, msg.Chat.ID, msg.ID, catID, "setgrp")
		return
	}

	// catmgr:topic:{catID} — pick a group then pick a topic
	if strings.HasPrefix(action, "topic:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "topic:"), 10, 64)
		if err != nil {
			return
		}
		h.showGroupPickerForCatMgr(ctx, b, msg.Chat.ID, msg.ID, catID, "settopicgrp")
		return
	}

	// catmgr:setgrp:{catID}:{chatID}
	if strings.HasPrefix(action, "setgrp:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "setgrp:"), ":", 2)
		if len(parts) != 2 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		chatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}
		if err := h.storage.UpdateCategoryScope(ctx, catID, &chatID, nil); err != nil {
			log.Printf("❌ UpdateCategoryScope: %v", err)
			return
		}
		cat, _ := h.storage.GetCategory(ctx, catID)
		if cat != nil {
			h.sendCategoryDetail(ctx, b, msg.Chat.ID, msg.ID, cat)
		}
		return
	}

	// catmgr:settopicgrp:{catID}:{chatID} — group chosen, now pick topic
	if strings.HasPrefix(action, "settopicgrp:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "settopicgrp:"), ":", 2)
		if len(parts) != 2 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		chatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}
		h.showTopicPickerForCatMgr(ctx, b, msg.Chat.ID, msg.ID, catID, chatID)
		return
	}

	// catmgr:settopic:{catID}:{chatID}:{threadID}
	if strings.HasPrefix(action, "settopic:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "settopic:"), ":", 3)
		if len(parts) != 3 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		chatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		threadID, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		if err := h.storage.UpdateCategoryScope(ctx, catID, &chatID, &threadID); err != nil {
			log.Printf("❌ UpdateCategoryScope: %v", err)
			return
		}
		cat, _ := h.storage.GetCategory(ctx, catID)
		if cat != nil {
			h.sendCategoryDetail(ctx, b, msg.Chat.ID, msg.ID, cat)
		}
		return
	}

	// catmgr:delete:{catID} — confirmation prompt
	if strings.HasPrefix(action, "delete:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "delete:"), 10, 64)
		if err != nil {
			return
		}
		cat, _ := h.storage.GetCategory(ctx, catID)
		catName := fmt.Sprintf("ID %d", catID)
		if cat != nil {
			catName = cat.Emoji + " " + cat.Name
		}
		rows := [][]models.InlineKeyboardButton{{
			{Text: h.trans.Category.ConfirmDelete, CallbackData: fmt.Sprintf("catmgr:delconfirm:%d", catID)},
			{Text: h.trans.Admin.Cancel, CallbackData: fmt.Sprintf("catmgr:detail:%d", catID)},
		}}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			Text:        fmt.Sprintf(h.trans.Category.ConfirmDeleteCategory, catName),
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		return
	}

	// catmgr:delconfirm:{catID}
	if strings.HasPrefix(action, "delconfirm:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "delconfirm:"), 10, 64)
		if err != nil {
			return
		}
		if err := h.storage.DeleteCategory(ctx, catID); err != nil {
			log.Printf("❌ DeleteCategory: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      fmt.Sprintf(h.trans.Error.FailedDelete, err),
			})
			return
		}
		h.sendCategoryList(ctx, b, msg.Chat.ID, msg.ID)
		return
	}

	// catmgr:editname:{catID}, catmgr:editemoji:{catID}, catmgr:editkey:{catID}
	if strings.HasPrefix(action, "editname:") || strings.HasPrefix(action, "editemoji:") || strings.HasPrefix(action, "editkey:") {
		var prefix, step, prompt string
		if strings.HasPrefix(action, "editname:") {
			prefix, step, prompt = "editname:", StepAdminCatName, h.trans.Category.EnterName
		} else if strings.HasPrefix(action, "editemoji:") {
			prefix, step, prompt = "editemoji:", StepAdminCatEmoji, h.trans.Category.EnterEmoji
		} else {
			prefix, step, prompt = "editkey:", StepAdminCatTeamKey, h.trans.Category.EnterLinearTeamKey
		}

		catID, err := strconv.ParseInt(strings.TrimPrefix(action, prefix), 10, 64)
		if err != nil {
			return
		}
		cat, err := h.storage.GetCategory(ctx, catID)
		if err != nil || cat == nil {
			return
		}

		key := stateKey{UserID: query.From.ID}
		h.mu.Lock()
		h.states[key] = &pendingAdminSession{
			Cmd:          AdminCmdEditCategory,
			Step:         step,
			MessageID:    msg.ID,
			ChatID:       msg.Chat.ID,
			CategoryID:   catID,
			CategoryName: cat.Name,
			TypeName:     cat.Emoji,
			TeamKey:      cat.LinearTeamKey,
			CreatedAt:    time.Now(),
		}
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      prompt,
		})
		return
	}

	// catmgr:clone:{catID} — pick target group
	if strings.HasPrefix(action, "clone:") {
		catID, err := strconv.ParseInt(strings.TrimPrefix(action, "clone:"), 10, 64)
		if err != nil {
			return
		}
		h.showGroupPickerForCatMgr(ctx, b, msg.Chat.ID, msg.ID, catID, "clonegrp")
		return
	}

	// catmgr:clonegrp:{catID}:{chatID} — group chosen, show topic picker or key step
	if strings.HasPrefix(action, "clonegrp:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "clonegrp:"), ":", 2)
		if len(parts) != 2 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		targetChatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}
		h.showCloneTopicPicker(ctx, b, msg.Chat.ID, msg.ID, catID, targetChatID)
		return
	}

	// catmgr:clonetopic:{catID}:{chatID}:{threadID} — scope chosen, show key step
	if strings.HasPrefix(action, "clonetopic:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "clonetopic:"), ":", 3)
		if len(parts) != 3 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		targetChatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		threadID, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		h.showCloneKeyStep(ctx, b, msg.Chat.ID, msg.ID, query.From.ID, catID, targetChatID, threadID)
		return
	}

	// catmgr:clonesame:{catID}:{chatID}:{threadID} — clone with same Linear key
	if strings.HasPrefix(action, "clonesame:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "clonesame:"), ":", 3)
		if len(parts) != 3 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		targetChatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		threadID, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		src, err := h.storage.GetCategory(ctx, catID)
		if err != nil {
			return
		}
		h.execClone(ctx, b, msg.Chat.ID, msg.ID, query.From.ID, catID, targetChatID, threadID, src.LinearTeamKey)
		return
	}

	// catmgr:clonekey:{catID}:{chatID}:{threadID} — change key before cloning
	if strings.HasPrefix(action, "clonekey:") {
		parts := strings.SplitN(strings.TrimPrefix(action, "clonekey:"), ":", 3)
		if len(parts) != 3 {
			return
		}
		catID, err1 := strconv.ParseInt(parts[0], 10, 64)
		targetChatID, err2 := strconv.ParseInt(parts[1], 10, 64)
		threadID, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		src, err := h.storage.GetCategory(ctx, catID)
		if err != nil {
			return
		}

		sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf(h.trans.Category.CloningPrompt, src.Emoji, src.Name, src.LinearTeamKey),
		})
		if err != nil {
			log.Printf("❌ clonekey SendMessage: %v", err)
			return
		}

		h.mu.Lock()
		h.states[stateKey{UserID: query.From.ID}] = &pendingAdminSession{
			Cmd:               AdminCmdCloneCategory,
			Step:              StepAdminCatTeamKey,
			MessageID:         sentMsg.ID,
			ChatID:            msg.Chat.ID,
			UserID:            query.From.ID,
			CategoryID:        catID,
			CategoryName:      src.Name,
			TeamKey:           src.LinearTeamKey,
			TargetGroupChatID: targetChatID,
			ThreadID:          threadID,
		}
		h.mu.Unlock()
		return
	}
}

func (h *Handler) showGroupPickerForCatMgr(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, catID int64, action string) {
	allGroups, err := h.storage.ListGroups(ctx)
	if err != nil {
		log.Printf("❌ ListGroups: %v", err)
		return
	}

	rows := make([][]models.InlineKeyboardButton, 0)
	for _, g := range allGroups {
		if !g.Approved {
			continue
		}
		title := g.Title
		if title == "" {
			title = fmt.Sprintf("Group %d", g.ChatID)
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         title,
			CallbackData: fmt.Sprintf("catmgr:%s:%d:%d", action, catID, g.ChatID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         h.trans.Admin.Back,
		CallbackData: fmt.Sprintf("catmgr:detail:%d", catID),
	}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   editMsgID,
		Text:        h.trans.Admin.SelectGroup,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// showCloneTopicPicker shows group-level option + all topics for clone target selection.
func (h *Handler) showCloneTopicPicker(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, catID int64, targetChatID int64) {
	groupName := h.getGroupName(targetChatID)
	topics := h.getTopics(targetChatID)

	rows := [][]models.InlineKeyboardButton{
		{{
			Text:         h.trans.Category.GroupLevel,
			CallbackData: fmt.Sprintf("catmgr:clonetopic:%d:%d:0", catID, targetChatID),
		}},
	}
	for _, t := range sortTopics(topics) {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf(h.trans.Category.TopicLevel, t.Name),
			CallbackData: fmt.Sprintf("catmgr:clonetopic:%d:%d:%d", catID, targetChatID, t.ThreadID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         h.trans.Admin.Back,
		CallbackData: fmt.Sprintf("catmgr:clone:%d", catID),
	}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   editMsgID,
		Text:        "📌 Clone to: " + groupName + "\n\nSelect scope:",
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// showCloneKeyStep shows the Linear key confirmation step before cloning.
func (h *Handler) showCloneKeyStep(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, userID int64, catID int64, targetChatID int64, threadID int) {
	src, err := h.storage.GetCategory(ctx, catID)
	if err != nil {
		return
	}

	rows := [][]models.InlineKeyboardButton{
		{{
			Text:         fmt.Sprintf(h.trans.Common.KeepOption, src.LinearTeamKey),
			CallbackData: fmt.Sprintf("catmgr:clonesame:%d:%d:%d", catID, targetChatID, threadID),
		}},
		{{
			Text:         h.trans.Category.EditKey,
			CallbackData: fmt.Sprintf("catmgr:clonekey:%d:%d:%d", catID, targetChatID, threadID),
		}},
		{{
			Text:         h.trans.Admin.Back,
			CallbackData: fmt.Sprintf("catmgr:clonegrp:%d:%d", catID, targetChatID),
		}},
	}
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   editMsgID,
		Text:        h.trans.Category.EnterNewTeamKey,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// execClone performs the actual clone and shows the result.
func (h *Handler) execClone(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, userID int64, catID int64, targetChatID int64, threadID int, teamKey string) {
	var scopeChatID *int64
	var scopeThreadID *int
	if targetChatID != 0 {
		scopeChatID = &targetChatID
	}
	if threadID != 0 {
		scopeThreadID = &threadID
	}

	newCatID, err := h.storage.CloneCategory(ctx, catID, scopeChatID, scopeThreadID, teamKey)
	if err != nil {
		log.Printf("❌ CloneCategory: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: editMsgID,
			Text:      fmt.Sprintf(h.trans.Error.ClonedFailed, err),
		})
		return
	}

	src, _ := h.storage.GetCategory(ctx, catID)
	catName := fmt.Sprintf("ID %d", catID)
	if src != nil {
		catName = src.Emoji + " " + src.Name
	}
	log.Printf("✓ Cloned category %d (%s) → new ID %d (team: %s)", catID, catName, newCatID, teamKey)

	// Clean up any clone session
	h.mu.Lock()
	delete(h.states, stateKey{UserID: userID})
	h.mu.Unlock()

	h.sendCategoryList(ctx, b, chatID, editMsgID)
}

func (h *Handler) showTopicPickerForCatMgr(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int, catID int64, targetChatID int64) {
	topics := h.getTopics(targetChatID)

	rows := make([][]models.InlineKeyboardButton, 0)
	for _, t := range sortTopics(topics) {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf(h.trans.Category.TopicLevel, t.Name),
			CallbackData: fmt.Sprintf("catmgr:settopic:%d:%d:%d", catID, targetChatID, t.ThreadID),
		}})
	}
	if len(rows) == 0 {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         h.trans.Admin.NoTopicsYet,
			CallbackData: fmt.Sprintf("catmgr:detail:%d", catID),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         h.trans.Admin.Back,
		CallbackData: fmt.Sprintf("catmgr:detail:%d", catID),
	}})

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   editMsgID,
		Text:        h.trans.Category.SelectTopic,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}
