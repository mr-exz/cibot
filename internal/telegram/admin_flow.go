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

// handleAdminPendingInput routes admin pending state messages to appropriate handlers
func (h *Handler) handleAdminPendingInput(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	switch admin.Cmd {
	case AdminCmdAddCategory:
		h.handleAdminAddCategoryPending(ctx, b, msg, admin)
	case AdminCmdAddType:
		h.handleAdminAddTypePending(ctx, b, msg, admin)
	case AdminCmdAddPerson:
		h.handleAdminAddPersonPending(ctx, b, msg, admin)
	case AdminCmdSetRotation:
		h.handleAdminSetRotationPending(ctx, b, msg, admin)
	case AdminCmdSetWorkHours:
		h.handleAdminSetWorkHoursPending(ctx, b, msg, admin)
	case AdminCmdAddTopic:
		h.handleAdminAddTopicPending(ctx, b, msg, admin)
	case AdminCmdSetLabel:
		h.handleAdminSetLabelPending(ctx, b, msg, admin)
	}
}

// ===== /addcategory flow =====

func (h *Handler) handleAdminAddCategoryPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	key := stateKey{UserID: msg.From.ID}

	switch admin.Step {
	case StepAdminCatName:
		admin.CategoryName = text
		admin.Step = StepAdminCatEmoji
		h.mu.Lock()
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      h.catProgressText(admin) + "\n\n😀 Enter emoji:",
		})

	case StepAdminCatEmoji:
		admin.TypeName = text
		admin.Step = StepAdminCatTeamKey
		h.mu.Lock()
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      h.catProgressText(admin) + "\n\n⌨️ Enter Linear team key (e.g., INFRA):",
		})

	case StepAdminCatTeamKey:
		admin.TeamKey = text
		h.addCategoryNow(ctx, b, msg.From.ID, admin)
	}
}

// catProgressText builds the context header shown during category creation steps
func (h *Handler) catProgressText(admin *pendingAdminSession) string {
	groupName := h.getGroupName(admin.TargetGroupChatID)
	scope := "🌐 Global"
	if admin.ThreadID != 0 {
		topics := h.getTopics(admin.TargetGroupChatID)
		if name, ok := topics[admin.ThreadID]; ok {
			scope = "📌 " + name
		}
	}
	lines := []string{
		fmt.Sprintf("🏘️ %s  ·  %s", groupName, scope),
	}
	if admin.CategoryName != "" {
		lines = append(lines, fmt.Sprintf("✓ Name: %s", admin.CategoryName))
	}
	if admin.TypeName != "" {
		lines = append(lines, fmt.Sprintf("✓ Emoji: %s", admin.TypeName))
	}
	if admin.TeamKey != "" {
		lines = append(lines, fmt.Sprintf("✓ Team key: %s", admin.TeamKey))
	}
	return strings.Join(lines, "\n")
}

func (h *Handler) addCategoryNow(ctx context.Context, b *tgbot.Bot, userID int64, admin *pendingAdminSession) {
	key := stateKey{UserID: userID}

	var chatID *int64
	var threadID *int
	if admin.ThreadID != 0 {
		// Use TargetGroupChatID if set (DM flow), otherwise ChatID (in-group flow)
		targetChatID := admin.TargetGroupChatID
		if targetChatID == 0 {
			targetChatID = admin.ChatID
		}
		chatID = &targetChatID
		threadID = &admin.ThreadID
	}

	catID, err := h.storage.AddCategoryWithTopic(ctx, admin.CategoryName, admin.TypeName, admin.TeamKey, chatID, threadID)
	if err != nil {
		log.Printf("❌ Failed to add category: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("❌ Failed to add category: %v", err),
		})
		return
	}

	h.mu.Lock()
	delete(h.states, key)
	h.mu.Unlock()

	var scopeMsg string
	if admin.ThreadID != 0 {
		scopeMsg = " for this topic"
	} else {
		scopeMsg = " globally"
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    admin.ChatID,
		MessageID: admin.MessageID,
		Text:      fmt.Sprintf("✅ **Category added%s!**\n\n%s %s → %s\n\n**ID:** %d\n📝 Use this ID for `/addtype` and `/addperson`", scopeMsg, admin.TypeName, admin.CategoryName, admin.TeamKey, catID),
	})

	log.Printf("✓ Admin added category: %s (ID: %d)", admin.CategoryName, catID)
}

// ===== /addtype flow =====

func (h *Handler) handleAdminAddTypePending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	if admin.Step == StepAdminTypeName {
		// Store type name and create
		admin.TypeName = text

		typeID, err := h.storage.AddRequestType(ctx, admin.TypeName)
		if err != nil {
			log.Printf("❌ Failed to add request type: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Failed to add request type: %v", err),
			})
			return
		}

		// Link to category
		if err := h.storage.LinkRequestTypeToCategory(ctx, admin.CategoryID, typeID); err != nil {
			log.Printf("❌ Failed to link type to category: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Failed to link type: %v", err),
			})
			return
		}

		// Clean up state
		key := stateKey{UserID: msg.From.ID}
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✅ **Request type added!**\n\n%s → %s", admin.CategoryName, text),
		})

		log.Printf("✓ Admin added type %s to category %d", admin.TypeName, admin.CategoryID)
	}
}

// ===== /addperson flow =====

func (h *Handler) handleAdminAddPersonPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch admin.Step {
	case StepAdminPersonName:
		admin.PersonName = text
		admin.Step = StepAdminPersonTelegram
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n\n📱 **Enter Telegram username (@...):**", admin.PersonName),
		})

	case StepAdminPersonTelegram:
		admin.TgUsername = strings.TrimPrefix(text, "@")
		admin.Step = StepAdminPersonLinear
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n✓ Telegram: @%s\n\n🔷 **Enter Linear username (@...):**", admin.PersonName, admin.TgUsername),
		})

	case StepAdminPersonLinear:
		admin.LinearUsername = strings.TrimPrefix(text, "@")
		admin.Step = StepAdminPersonTimezone
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		keyboard := buildSkipKeyboard()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      admin.ChatID,
			MessageID:   admin.MessageID,
			Text:        fmt.Sprintf("✓ Name: %s\n✓ Telegram: @%s\n✓ Linear: @%s\n\n🌍 **Enter timezone (e.g., +02:00) or skip:**", admin.PersonName, admin.TgUsername, admin.LinearUsername),
			ReplyMarkup: keyboard,
		})

	case StepAdminPersonTimezone:
		if text != "" && text != "skip" {
			admin.Timezone = text
		}
		admin.Step = StepAdminPersonHours
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		keyboard := buildSkipKeyboard()
		tzDisplay := admin.Timezone
		if tzDisplay == "" {
			tzDisplay = "(skipped)"
		}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      admin.ChatID,
			MessageID:   admin.MessageID,
			Text:        fmt.Sprintf("✓ Timezone: %s\n\n⏰ **Enter work hours (e.g., 08:30-18:30) or skip:**", tzDisplay),
			ReplyMarkup: keyboard,
		})

	case StepAdminPersonHours:
		if text != "" && text != "skip" {
			admin.WorkHours = text
		}
		admin.Step = StepAdminPersonDays
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		hoursDisplay := admin.WorkHours
		if hoursDisplay == "" {
			hoursDisplay = "(skipped)"
		}
		keyboard := buildSkipKeyboard()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      admin.ChatID,
			MessageID:   admin.MessageID,
			Text:        fmt.Sprintf("✓ Work hours: %s\n\n📅 **Enter work days (e.g., 1-5) or skip:**", hoursDisplay),
			ReplyMarkup: keyboard,
		})

	case StepAdminPersonDays:
		if text != "" && text != "skip" {
			admin.WorkDays = text
		}

		// Validate work hours format if provided
		if admin.WorkHours != "" {
			if _, _, err := storage.ParseWorkHours(admin.WorkHours); err != nil {
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("❌ Invalid work hours format: %v", err),
				})
				return
			}
		}

		// Validate work days format if provided
		if admin.WorkDays != "" {
			if _, err := storage.ParseWorkDays(admin.WorkDays); err != nil {
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("❌ Invalid work days format: %v", err),
				})
				return
			}
		}

		// Validate timezone format if provided
		if admin.Timezone != "" {
			if _, err := storage.ParseTimezone(admin.Timezone); err != nil {
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("❌ Invalid timezone format: %v", err),
				})
				return
			}
		}

		// Create person
		personID, err := h.storage.AddSupportPersonFull(ctx, admin.PersonName, admin.TgUsername, admin.LinearUsername, admin.Timezone, admin.WorkHours, admin.WorkDays)
		if err != nil {
			log.Printf("❌ Failed to add support person: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Failed to add person: %v", err),
			})
			return
		}

		// Create initial assignment
		startDate := time.Now().Format("2006-01-02")
		if err := h.storage.CreateInitialAssignment(ctx, admin.CategoryID, personID, "daily", startDate); err != nil {
			log.Printf("❌ Failed to create assignment: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Failed to create assignment: %v", err),
			})
			return
		}

		// Clean up state
		key := stateKey{UserID: msg.From.ID}
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()

		daysDisplay := admin.WorkDays
		if daysDisplay == "" {
			daysDisplay = "(none)"
		}

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✅ **Support person added!**\n\n👤 %s\n🔵 @%s | 🔷 @%s\n⏰ %s\n📅 %s", admin.PersonName, admin.TgUsername, admin.LinearUsername, admin.WorkHours, daysDisplay),
		})

		log.Printf("✓ Admin added support person %s", admin.PersonName)
	}
}

// ===== /setrotation flow (reuses existing callbacks) =====

func (h *Handler) handleAdminSetRotationPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	// Not used - setrotation uses callbacks for type selection
}

// ===== /setworkhours flow =====

func (h *Handler) handleAdminSetWorkHoursPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch admin.Step {
	case StepAdminWhTimezone:
		admin.Timezone = text
		admin.Step = StepAdminWhHours
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Timezone: %s\n\n⏰ **Enter work hours (e.g., 08:30-18:30):**", admin.Timezone),
		})

	case StepAdminWhHours:
		admin.WorkHours = text
		admin.Step = StepAdminWhDays
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Work hours: %s\n\n📅 **Enter work days (e.g., 1-5 or 12345):**", admin.WorkHours),
		})

	case StepAdminWhDays:
		admin.WorkDays = text

		// Validate formats
		if _, _, err := storage.ParseWorkHours(admin.WorkHours); err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Invalid work hours format: %v", err),
			})
			return
		}

		if _, err := storage.ParseWorkDays(admin.WorkDays); err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Invalid work days format: %v", err),
			})
			return
		}

		if _, err := storage.ParseTimezone(admin.Timezone); err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Invalid timezone format: %v", err),
			})
			return
		}

		// Update person
		err := h.storage.SetPersonWorkHours(ctx, admin.TgUsername, admin.Timezone, admin.WorkHours, admin.WorkDays)
		if err != nil {
			log.Printf("❌ Failed to set work hours: %v", err)
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("❌ Failed to set work hours: %v", err),
			})
			return
		}

		// Clean up state
		key := stateKey{UserID: msg.From.ID}
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✅ **Work hours updated!**\n\n🌍 Timezone: %s\n⏰ Hours: %s\n📅 Days: %s", admin.Timezone, admin.WorkHours, admin.WorkDays),
		})

		log.Printf("✓ Admin updated work hours for @%s", admin.TgUsername)
	}
}

// ===== Category selection for admin flows =====

func (h *Handler) handleAdminCategoryCallback(ctx context.Context, b *tgbot.Bot, admin *pendingAdminSession, cat *storage.Category) {
	admin.CategoryID = cat.ID
	admin.CategoryName = cat.Name
	admin.TeamKey = cat.LinearTeamKey

	switch admin.Cmd {
	case AdminCmdAddType:
		// Transition to asking for type name
		admin.Step = StepAdminTypeName
		h.mu.Lock()
		key := stateKey{UserID: admin.UserID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Category: %s %s\n\n📝 **Enter request type name:**", cat.Emoji, cat.Name),
		})

	case AdminCmdAddPerson:
		// Transition to asking for person name
		admin.Step = StepAdminPersonName
		h.mu.Lock()
		key := stateKey{UserID: admin.UserID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Category: %s %s\n\n👤 **Enter support person name:**", cat.Emoji, cat.Name),
		})

	case AdminCmdSetRotation:
		// Transition to rotation type selection
		admin.Step = StepAdminSelectRotationType
		h.mu.Lock()
		key := stateKey{UserID: admin.UserID}
		h.states[key] = admin
		h.mu.Unlock()

		keyboard := buildRotationTypeKeyboard()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      admin.ChatID,
			MessageID:   admin.MessageID,
			Text:        fmt.Sprintf("✓ Category: %s %s\n\n📅 **Select rotation type:**", cat.Emoji, cat.Name),
			ReplyMarkup: keyboard,
		})
	}
}

// ===== Hierarchical category picker for admin flows =====

// startAdminCategoryPicker sends the top-level hierarchical category keyboard.
// Used by /addtype, /addperson, /setrotation.
func (h *Handler) startAdminCategoryPicker(ctx context.Context, b *tgbot.Bot, msg *models.Message, cmd AdminCmd) {
	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}
	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "❌ No categories yet. Create one with /addcategory")
		return
	}

	keyboard := h.buildAdminCatTopKeyboard(categories)
	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "🗂️ Select category:",
		ReplyMarkup:     keyboard,
		MessageThreadID: msg.MessageThreadID,
	})
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return
	}

	h.mu.Lock()
	h.states[stateKey{UserID: msg.From.ID}] = &pendingAdminSession{
		Cmd:       cmd,
		Step:      StepCategory,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()
	log.Printf("✓ Started /%s flow for %s", cmd, msg.From.Username)
}

// buildAdminCatTopKeyboard builds the top-level category picker:
// global categories as direct buttons, groups as navigation buttons.
func (h *Handler) buildAdminCatTopKeyboard(categories []storage.Category) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0)

	// Global categories first (no group/topic scope)
	for _, cat := range categories {
		if cat.ChatID == nil {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         cat.Emoji + " " + cat.Name,
				CallbackData: fmt.Sprintf("cat:%d", cat.ID),
			}})
		}
	}

	// Groups that have at least one scoped category
	seenGroups := make(map[int64]bool)
	for _, cat := range categories {
		if cat.ChatID != nil && !seenGroups[*cat.ChatID] {
			seenGroups[*cat.ChatID] = true
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "🏘️ " + h.getGroupName(*cat.ChatID),
				CallbackData: fmt.Sprintf("acat_grp:%d", *cat.ChatID),
			}})
		}
	}

	rows = append(rows, []models.InlineKeyboardButton{{Text: "❌ Cancel", CallbackData: "cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildAdminCatGroupKeyboard builds group-level category picker:
// group-scoped categories + topics that have scoped categories.
func (h *Handler) buildAdminCatGroupKeyboard(chatID int64, categories []storage.Category) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0)

	// Group-level categories (scoped to group but not a specific topic)
	for _, cat := range categories {
		if cat.ChatID != nil && *cat.ChatID == chatID && cat.ThreadID == nil {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         cat.Emoji + " " + cat.Name,
				CallbackData: fmt.Sprintf("cat:%d", cat.ID),
			}})
		}
	}

	// Topics that have topic-scoped categories in this group
	topics := h.getTopics(chatID)
	seenTopics := make(map[int]bool)
	for _, cat := range categories {
		if cat.ChatID != nil && *cat.ChatID == chatID && cat.ThreadID != nil && !seenTopics[*cat.ThreadID] {
			seenTopics[*cat.ThreadID] = true
			topicName := topics[*cat.ThreadID]
			if topicName == "" {
				topicName = fmt.Sprintf("Topic %d", *cat.ThreadID)
			}
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "📌 " + topicName,
				CallbackData: fmt.Sprintf("acat_topic:%d:%d", chatID, *cat.ThreadID),
			}})
		}
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "⬅️ Back", CallbackData: "acat_back"},
		{Text: "❌ Cancel", CallbackData: "cancel"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildAdminCatTopicKeyboard builds topic-level category picker.
func (h *Handler) buildAdminCatTopicKeyboard(chatID int64, threadID int, categories []storage.Category) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0)

	for _, cat := range categories {
		if cat.ChatID != nil && *cat.ChatID == chatID && cat.ThreadID != nil && *cat.ThreadID == threadID {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         cat.Emoji + " " + cat.Name,
				CallbackData: fmt.Sprintf("cat:%d", cat.ID),
			}})
		}
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "⬅️ Back", CallbackData: fmt.Sprintf("acat_grp:%d", chatID)},
		{Text: "❌ Cancel", CallbackData: "cancel"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// handleAdminCatGrpNav handles tapping a group button in the category picker.
func (h *Handler) handleAdminCatGrpNav(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	admin, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()
	if !ok || admin == nil || admin.Step != StepCategory {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	chatID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "acat_grp:"), 10, 64)
	if err != nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		return
	}

	keyboard := h.buildAdminCatGroupKeyboard(chatID, categories)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        "🏘️ " + h.getGroupName(chatID) + " — select category:",
		ReplyMarkup: keyboard,
	})
}

// handleAdminCatTopicNav handles tapping a topic button in the category picker.
func (h *Handler) handleAdminCatTopicNav(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	admin, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()
	if !ok || admin == nil || admin.Step != StepCategory {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	parts := strings.SplitN(strings.TrimPrefix(query.Data, "acat_topic:"), ":", 2)
	if len(parts) != 2 {
		return
	}
	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	threadID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		return
	}

	topics := h.getTopics(chatID)
	topicName := topics[threadID]
	if topicName == "" {
		topicName = fmt.Sprintf("Topic %d", threadID)
	}

	keyboard := h.buildAdminCatTopicKeyboard(chatID, threadID, categories)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        "🏘️ " + h.getGroupName(chatID) + "  ·  📌 " + topicName + " — select category:",
		ReplyMarkup: keyboard,
	})
}

// handleAdminCatBack handles the ⬅️ Back button — returns to the top-level picker.
func (h *Handler) handleAdminCatBack(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	admin, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()
	if !ok || admin == nil || admin.Step != StepCategory {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	categories, err := h.storage.ListCategories(ctx)
	if err != nil {
		return
	}

	keyboard := h.buildAdminCatTopKeyboard(categories)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        "🗂️ Select category:",
		ReplyMarkup: keyboard,
	})
}

// ===== Callback handlers for admin flows =====

func (h *Handler) handleAdminConfirmCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	msg := query.Message.Message
	if msg == nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	// Parse confirm type
	confirmType := strings.TrimPrefix(query.Data, "confirm:")

	if confirmType == "global" && adminPending.Step == StepAdminCatSelectTopic {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "✓ Global scope",
		})
		adminPending.ThreadID = 0
		adminPending.Step = StepAdminCatName
		h.mu.Lock()
		h.states[key] = adminPending
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    adminPending.ChatID,
			MessageID: adminPending.MessageID,
			Text:      h.catProgressText(adminPending) + "\n\n📝 Enter category name:",
		})
	} else {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	}
}

func (h *Handler) handleAdminTopicManualCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	// Parse topic callback data: "topic:{chatID}:{threadID}" — user selected a topic from the list
	topicData := strings.TrimPrefix(query.Data, "topic:")
	if topicData == "manual" {
		// No longer used in new flow
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return
	}

	// Format: "{chatID}:{threadID}" — user selected from the topic list
	parts := strings.SplitN(topicData, ":", 2)
	if len(parts) != 2 {
		return
	}
	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	threadID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ Topic selected",
	})

	adminPending.TargetGroupChatID = chatID
	adminPending.ThreadID = threadID

	if adminPending.Step != StepAdminCatSelectTopic {
		return
	}
	// New flow: topic chosen, proceed to name entry
	adminPending.Step = StepAdminCatName
	h.mu.Lock()
	h.states[key] = adminPending
	h.mu.Unlock()
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    adminPending.ChatID,
		MessageID: adminPending.MessageID,
		Text:      h.catProgressText(adminPending) + "\n\n📝 Enter category name:",
	})
}

func (h *Handler) handleAdminRotationCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	rotationType := strings.TrimPrefix(query.Data, "rot:")

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ Setting rotation...",
	})

	// Set rotation
	if err := h.storage.SetRotation(ctx, adminPending.CategoryID, rotationType); err != nil {
		log.Printf("❌ Failed to set rotation: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    adminPending.ChatID,
			MessageID: adminPending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to set rotation: %v", err),
		})
		return
	}

	// Clean up state
	h.mu.Lock()
	delete(h.states, key)
	h.mu.Unlock()

	rotationName := "Daily"
	if rotationType == "weekly" {
		rotationName = "Weekly"
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    adminPending.ChatID,
		MessageID: adminPending.MessageID,
		Text:      fmt.Sprintf("✅ **Rotation updated!**\n\n%s %s → %s", rotationName, rotationType, adminPending.CategoryName),
	})

	log.Printf("✓ Admin set rotation for category %d to %s", adminPending.CategoryID, rotationType)
}

func (h *Handler) handleAdminPersonCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	personIDStr := strings.TrimPrefix(query.Data, "person:")
	personID, err := strconv.ParseInt(personIDStr, 10, 64)
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	// Store person ID and username for later
	adminPending.PersonID = personID

	// Get person details to store username
	persons, _ := h.storage.ListAllSupportPersons(ctx)
	for _, p := range persons {
		if p.ID == personID {
			adminPending.TgUsername = p.TelegramUsername
			break
		}
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ Selected",
	})

	// Move to timezone step
	adminPending.Step = StepAdminWhTimezone
	h.mu.Lock()
	h.states[key] = adminPending
	h.mu.Unlock()

	keyboard := buildSkipKeyboard()
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      adminPending.ChatID,
		MessageID:   adminPending.MessageID,
		Text:        fmt.Sprintf("✓ Person selected\n\n🌍 **Enter timezone (e.g., +02:00):**"),
		ReplyMarkup: keyboard,
	})
}

// ===== /addtopic flow =====

func (h *Handler) handleAdminTopicGroupCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	chatIDStr := strings.TrimPrefix(query.Data, "grp:")
	selectedChatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "✓ Group selected",
	})

	// Handle addcategory group selection (new flow)
	if adminPending.Cmd == AdminCmdAddCategory && adminPending.Step == StepAdminCatSelectGroup {
		adminPending.TargetGroupChatID = selectedChatID
		topics := h.getTopics(selectedChatID)

		if len(topics) > 0 {
			// Group has topics — ask which one
			adminPending.Step = StepAdminCatSelectTopic
			h.mu.Lock()
			h.states[key] = adminPending
			h.mu.Unlock()

			rows := make([][]models.InlineKeyboardButton, 0)
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "🌐 Global (all topics)",
				CallbackData: "confirm:global",
			}})
			for threadID, topicName := range topics {
				rows = append(rows, []models.InlineKeyboardButton{{
					Text:         "📌 " + topicName,
					CallbackData: fmt.Sprintf("topic:%d:%d", selectedChatID, threadID),
				}})
			}
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "❌ Cancel",
				CallbackData: "cancel",
			}})

			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      adminPending.ChatID,
				MessageID:   adminPending.MessageID,
				Text:        fmt.Sprintf("🏘️ %s\n\n📌 Select topic:", h.getGroupName(selectedChatID)),
				ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
			})
		} else {
			// No topics registered — global scope, go straight to name
			adminPending.ThreadID = 0
			adminPending.Step = StepAdminCatName
			h.mu.Lock()
			h.states[key] = adminPending
			h.mu.Unlock()

			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    adminPending.ChatID,
				MessageID: adminPending.MessageID,
				Text:      fmt.Sprintf("🏘️ %s  ·  🌐 Global\n\n📝 Enter category name:", h.getGroupName(selectedChatID)),
			})
		}
		return
	}

	if adminPending.Cmd == AdminCmdSetLabel && adminPending.Step == StepAdminSetLabelGroup {
		if err := setChatMemberTag(ctx, h.cfg.TelegramToken, selectedChatID, adminPending.LabelUserID, adminPending.LabelText); err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    adminPending.ChatID,
				MessageID: adminPending.MessageID,
				Text:      fmt.Sprintf("❌ Failed to set tag: %v", err),
			})
		} else {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    adminPending.ChatID,
				MessageID: adminPending.MessageID,
				Text:      fmt.Sprintf("✅ Tag *%s* set for @%s", adminPending.LabelText, adminPending.LabelUsername),
			})
			log.Printf("✓ Tag set for user %d (@%s) in chat %d: %s", adminPending.LabelUserID, adminPending.LabelUsername, selectedChatID, adminPending.LabelText)
		}
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		return
	}

	if adminPending.Cmd != AdminCmdAddTopic {
		return
	}

	adminPending.SelectedChatID = selectedChatID
	adminPending.Step = StepAdminTopicName
	h.mu.Lock()
	h.states[key] = adminPending
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    adminPending.ChatID,
		MessageID: adminPending.MessageID,
		Text:      "📝 Enter topic name:",
	})
}

func (h *Handler) handleAdminAddTopicPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch admin.Step {
	case StepAdminTopicName:
		admin.TopicName = text
		admin.Step = StepAdminTopicID
		h.mu.Lock()
		h.states[stateKey{UserID: msg.From.ID}] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n\n🔢 Enter topic ID (the thread ID number):", admin.TopicName),
		})

	case StepAdminTopicID:
		topicID, err := strconv.Atoi(text)
		if err != nil || topicID <= 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("✓ Name: %s\n\n❌ Invalid topic ID. Enter a positive number:", admin.TopicName),
			})
			return
		}

		h.recordTopic(admin.SelectedChatID, topicID, admin.TopicName)

		h.mu.Lock()
		delete(h.states, stateKey{UserID: msg.From.ID})
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✅ Topic registered!\n\n🔹 #%d — %s\n\nNow available in /addcategory", topicID, admin.TopicName),
		})

		log.Printf("✓ Topic #%d registered for chat %d: %s", topicID, admin.SelectedChatID, admin.TopicName)
	}
}

func (h *Handler) handleAdminSkipCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	adminPending, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || adminPending == nil {
		return
	}

	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "⏭ Skipped",
	})

	// Create a mock message with "skip" text
	mockMsg := &models.Message{
		Text: "skip",
		From: &query.From,
	}

	// Re-trigger the pending handler as if user typed "skip"
	h.handleAdminPendingInput(ctx, b, mockMsg, adminPending)
}

// ===== /setlabel flow =====

func (h *Handler) handleAdminSetLabelPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	log.Printf("🏷 setlabel pending: step=%s user=%d text=%q", admin.Step, admin.LabelUserID, msg.Text)
	if admin.Step != StepAdminSetLabelWaitLabel {
		return
	}

	label := strings.TrimSpace(msg.Text)
	if label == "" || len([]rune(label)) > 16 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      "❌ Label must be 1–16 characters.",
		})
		return
	}

	allGroups, err := h.storage.ListGroups(ctx)
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("❌ Failed to load groups: %v", err),
		})
		return
	}
	approvedGroups := make(map[int64]string)
	for _, g := range allGroups {
		if g.Approved {
			approvedGroups[g.ChatID] = g.Title
		}
	}
	if len(approvedGroups) == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      "❌ No approved groups yet. Approve groups via /groups first.",
		})
		h.mu.Lock()
		delete(h.states, stateKey{UserID: admin.UserID})
		h.mu.Unlock()
		return
	}

	admin.LabelText = label
	admin.Step = StepAdminSetLabelGroup
	h.mu.Lock()
	h.states[stateKey{UserID: admin.UserID}] = admin
	h.mu.Unlock()

	_, err = b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        fmt.Sprintf("✓ Label: %s\n\n🏘 Select the group:", label),
		ReplyMarkup: buildGroupKeyboard(approvedGroups),
	})
	if err != nil {
		log.Printf("❌ setlabel EditMessageText failed: %v", err)
	} else {
		log.Printf("✓ setlabel group keyboard shown for label=%q user=%d", label, admin.LabelUserID)
	}
}
