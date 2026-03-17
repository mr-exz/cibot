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

// handleSetLabel sets a label for a Telegram user: /setlabel @username label text
func (h *Handler) handleSetLabel(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Parse: /setlabel @username label text
	parts := strings.Fields(msg.Text)
	if len(parts) < 3 || !strings.HasPrefix(parts[1], "@") {
		h.sendMessage(ctx, b, msg, "Usage: /setlabel @username label")
		return
	}

	username := strings.TrimPrefix(parts[1], "@")
	label := strings.Join(parts[2:], " ")

	if err := h.storage.SetUserLabel(ctx, username, label); err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to set label: %v", err))
		return
	}

	log.Printf("✓ Label set for @%s: %s (by %s)", username, label, msg.From.Username)
	h.sendMessage(ctx, b, msg, fmt.Sprintf("✅ Label for @%s set to: %s", username, label))
}

// handleAddCategory starts the /addcategory interactive flow
func (h *Handler) handleAddCategory(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	log.Printf("📝 /addcategory from %s (chat_id: %d, thread_id: %d)", msg.From.Username, msg.Chat.ID, msg.MessageThreadID)

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "📝 **Enter category name:**",
		MessageThreadID: msg.MessageThreadID,
		ParseMode:       models.ParseModeMarkdown,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdAddCategory,
		Step:      StepAdminCatName,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /addcategory flow for %s (thread_id=%d)", msg.From.Username, msg.MessageThreadID)
}

// handleAddType starts the /addtype interactive flow
func (h *Handler) handleAddType(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Get categories to show selection
	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}

	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No categories available. Create one first with /addcategory")
		return
	}

	keyboard := buildCategoryKeyboard(categories)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🗂️ **Select category:**",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
		ParseMode:       models.ParseModeMarkdown,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdAddType,
		Step:      StepCategory,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /addtype flow for %s", msg.From.Username)
}

// handleAddPerson starts the /addperson interactive flow
func (h *Handler) handleAddPerson(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Get categories to show selection
	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}

	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No categories available. Create one first with /addcategory")
		return
	}

	keyboard := buildCategoryKeyboard(categories)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🗂️ **Select category:**",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
		ParseMode:       models.ParseModeMarkdown,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdAddPerson,
		Step:      StepCategory,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /addperson flow for %s", msg.From.Username)
}

// handleSetRotation starts the /setrotation interactive flow
func (h *Handler) handleSetRotation(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	// Get categories to show selection
	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}

	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No categories available. Create one first with /addcategory")
		return
	}

	keyboard := buildCategoryKeyboard(categories)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🗂️ **Select category:**",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
		ParseMode:       models.ParseModeMarkdown,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdSetRotation,
		Step:      StepCategory,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		ThreadID:  msg.MessageThreadID,
		UserID:    msg.From.ID,
	}
	h.mu.Unlock()

	log.Printf("✓ Started /setrotation flow for %s", msg.From.Username)
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
		Text:            "👤 **Select person:**",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
		ParseMode:       models.ParseModeMarkdown,
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
	groups := h.getKnownGroups()
	if len(groups) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No groups discovered yet. Send any message in the target group first so the bot sees it.")
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
		sb.WriteString(fmt.Sprintf("\n%s:\n", groupName))
		for threadID, topicName := range topics {
			sb.WriteString(fmt.Sprintf("  🔹 #%d — %s\n", threadID, topicName))
		}
	}

	sb.WriteString("\nUse /addcategory to link categories to topics.")
	h.sendMessage(ctx, b, msg, sb.String())
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
	response = "📋 **Current On-Duty Support**\n\n"

	for _, duty := range duties {
		status := "🟢"
		if !duty.Online {
			status = "🔴"
		}
		response += fmt.Sprintf("%s **%s** → %s %s\n  🔵 @%s | 🔷 @%s\n\n",
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
