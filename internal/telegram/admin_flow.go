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

	// Handle manual topic ID entry (from DM)
	if admin.Step == StepAdminCatManualTopicID {
		topicID, err := strconv.Atoi(text)
		if err != nil || topicID < 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      "❌ Invalid topic ID. Must be a positive number.",
			})
			return
		}

		admin.ThreadID = topicID
		h.addCategoryNow(ctx, b, msg.From.ID, admin)
		return
	}

	switch admin.Step {
	case StepAdminCatName:
		// Store name and ask for emoji
		admin.CategoryName = text
		admin.Step = StepAdminCatEmoji
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n\n😀 **Enter emoji:**", text),
		})

	case StepAdminCatEmoji:
		// Store emoji and ask for team key
		admin.TypeName = text // Temporarily use TypeName to store emoji
		admin.Step = StepAdminCatTeamKey
		h.mu.Lock()
		key := stateKey{UserID: msg.From.ID}
		h.states[key] = admin
		h.mu.Unlock()

		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n✓ Emoji: %s\n\n⌨️ **Enter Linear team key (e.g., INFRA):**", admin.CategoryName, text),
		})

	case StepAdminCatTeamKey:
		// Store team key and ask for topic confirmation/selection
		admin.TeamKey = text

		if admin.ThreadID != 0 {
			// In a topic - ask for confirmation
			admin.Step = StepCategory // Reuse for keyboard step
			h.mu.Lock()
			key := stateKey{UserID: msg.From.ID}
			h.states[key] = admin
			h.mu.Unlock()

			keyboard := buildTopicConfirmKeyboard()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      admin.ChatID,
				MessageID:   admin.MessageID,
				Text:        fmt.Sprintf("✓ Name: %s\n✓ Emoji: %s\n✓ Team key: %s\n\n🗂️ **This category will be linked to the current topic. Confirm?**", admin.CategoryName, admin.TypeName, admin.TeamKey),
				ReplyMarkup: keyboard,
			})
		} else {
			// In DM - ask to select topic or make global
			admin.Step = "admin_select_topic"
			h.mu.Lock()
			key := stateKey{UserID: msg.From.ID}
			h.states[key] = admin
			h.mu.Unlock()

			// Load topics from ALL known groups (not just DM chat)
			allTopics := h.getAllTopics()
			rows := make([][]models.InlineKeyboardButton, 0)

			// Global option first
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "🌐 Make global (all topics)",
				CallbackData: "confirm:global",
			}})

			// Topic buttons — encode chatID:threadID so we know which group
			multiGroup := len(allTopics) > 1
			for chatID, topics := range allTopics {
				groupName := h.getGroupName(chatID)
				for threadID, topicName := range topics {
					label := topicName
					if multiGroup {
						label = fmt.Sprintf("%s  ·  %s", topicName, groupName)
					}
					rows = append(rows, []models.InlineKeyboardButton{{
						Text:         "📌 " + label,
						CallbackData: fmt.Sprintf("topic:%d:%d", chatID, threadID),
					}})
				}
			}

			// Manual entry option
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "📝 Enter topic ID manually",
				CallbackData: "topic:manual",
			}})

			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      admin.ChatID,
				MessageID:   admin.MessageID,
				Text:        fmt.Sprintf("✓ Name: %s\n✓ Emoji: %s\n✓ Team key: %s\n\n🗂️ Link to a topic or make global:", admin.CategoryName, admin.TypeName, admin.TeamKey),
				ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
			})
		}
	}
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

	if confirmType == "topic" {
		// Link to current topic
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "✓ Confirmed",
		})
		h.addCategoryNow(ctx, b, query.From.ID, adminPending)
	} else if confirmType == "global" {
		// Make global (no topic)
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "✓ Confirmed",
		})
		adminPending.ThreadID = 0
		h.addCategoryNow(ctx, b, query.From.ID, adminPending)
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

	// Parse topic callback data: "topic:manual" or "topic:123" (topic ID)
	topicData := strings.TrimPrefix(query.Data, "topic:")

	if topicData == "manual" {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Enter topic ID",
		})

		groups := h.getKnownGroups()
		if len(groups) == 1 {
			// Only one group — skip group selection, go straight to ID entry
			for chatID := range groups {
				adminPending.TargetGroupChatID = chatID
			}
			adminPending.Step = StepAdminCatManualTopicID
			h.mu.Lock()
			h.states[key] = adminPending
			h.mu.Unlock()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    adminPending.ChatID,
				MessageID: adminPending.MessageID,
				Text:      fmt.Sprintf("✓ Name: %s\n✓ Emoji: %s\n✓ Team key: %s\n\n🔢 Enter the forum topic ID:", adminPending.CategoryName, adminPending.TypeName, adminPending.TeamKey),
			})
		} else {
			// Multiple groups — ask which group first
			adminPending.Step = StepAdminCatManualGroup
			h.mu.Lock()
			h.states[key] = adminPending
			h.mu.Unlock()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      adminPending.ChatID,
				MessageID:   adminPending.MessageID,
				Text:        "🏘️ Select the group this topic belongs to:",
				ReplyMarkup: buildGroupKeyboard(groups),
			})
		}
	} else {
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
		h.addCategoryNow(ctx, b, query.From.ID, adminPending)
	}
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

	// Handle addcategory manual group selection
	if adminPending.Cmd == AdminCmdAddCategory && adminPending.Step == StepAdminCatManualGroup {
		adminPending.TargetGroupChatID = selectedChatID
		adminPending.Step = StepAdminCatManualTopicID
		h.mu.Lock()
		h.states[key] = adminPending
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    adminPending.ChatID,
			MessageID: adminPending.MessageID,
			Text:      fmt.Sprintf("✓ Name: %s\n✓ Emoji: %s\n✓ Team key: %s\n\n🔢 Enter the forum topic ID:", adminPending.CategoryName, adminPending.TypeName, adminPending.TeamKey),
		})
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

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      admin.ChatID,
		MessageID:   admin.MessageID,
		Text:        fmt.Sprintf("✓ Label: %s\n\n🏘 Select the group:", label),
		ReplyMarkup: buildGroupKeyboard(approvedGroups),
	})
}
