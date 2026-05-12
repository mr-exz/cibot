package telegram

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/storage"
)

type reminderEntry struct {
	person       *storage.SupportPerson
	categoryName string
}

type scheduledReminder struct {
	fireAt   time.Time
	chatID   int64
	threadID int // 0 = group-level, no topic
	entries  []reminderEntry
}

// startReminderScheduler runs in the background, computing and scheduling daily reminders.
// It recomputes on startup, on a signal from reminderCh, and every day at midnight UTC.
func (h *Handler) startReminderScheduler(ctx context.Context, b *tgbot.Bot) {
	h.scheduleRemindersForToday(ctx, b)

	for {
		nextMidnight := nextMidnightUTC()
		select {
		case <-ctx.Done():
			return
		case <-h.reminderCh:
		case <-time.After(time.Until(nextMidnight)):
		}
		h.scheduleRemindersForToday(ctx, b)
	}
}

func (h *Handler) scheduleRemindersForToday(ctx context.Context, b *tgbot.Bot) {
	// Stop and discard any previously scheduled timers before rescheduling.
	// This prevents double-firing if scheduleRemindersForToday is called more than once per day.
	for key, t := range h.reminderTimers {
		t.Stop()
		delete(h.reminderTimers, key)
	}

	enabled, err := h.storage.GetReminderEnabled(ctx)
	if err != nil {
		log.Printf("⚠️ reminder: failed to check enabled state: %v", err)
		return
	}
	if !enabled {
		return
	}

	now := time.Now()
	reminders, err := h.buildReminderPlan(ctx, now)
	if err != nil {
		log.Printf("⚠️ reminder: failed to build plan: %v", err)
		return
	}

	count := 0
	for _, r := range reminders {
		if !r.fireAt.After(now) {
			continue
		}
		r := r
		key := fmt.Sprintf("%d:%d:%s", r.chatID, r.threadID, r.fireAt.Format(time.RFC3339))
		h.reminderTimers[key] = time.AfterFunc(r.fireAt.Sub(now), func() {
			enabled, _ := h.storage.GetReminderEnabled(context.Background())
			if !enabled {
				return
			}
			h.fireReminder(context.Background(), b, r)
		})
		count++
	}
	if count > 0 {
		log.Printf("✓ reminder: %d reminder(s) scheduled for today", count)
	}
}

// buildReminderPlan returns the list of reminders to fire on the calendar day of forDate.
// Reminders are grouped by destination (chatID + threadID) and fire time.
func (h *Handler) buildReminderPlan(ctx context.Context, forDate time.Time) ([]scheduledReminder, error) {
	rotations, err := h.storage.ListAllRotations(ctx, forDate)
	if err != nil {
		return nil, err
	}

	type destKey struct {
		chatID   int64
		threadID int
		fireAt   time.Time
	}
	grouped := make(map[destKey][]reminderEntry)

	for _, rot := range rotations {
		if rot.Category.ChatID == nil || rot.OnDuty == nil {
			continue
		}
		person := rot.OnDuty
		if person.WorkHours == "" {
			continue
		}

		startMin, _, err := storage.ParseWorkHours(person.WorkHours)
		if err != nil {
			continue
		}

		loc := time.UTC
		if person.Timezone != "" {
			if parsed, err := storage.ParseTimezone(person.Timezone); err == nil {
				loc = parsed
			}
		}

		// Compute the shift-start moment on the same calendar date as forDate, in the person's timezone
		y, m, d := forDate.In(loc).Date()
		shiftStart := time.Date(y, m, d, startMin/60, startMin%60, 0, 0, loc)
		fireAt := shiftStart.UTC().Truncate(time.Minute)

		chatID := *rot.Category.ChatID
		threadID := 0
		if rot.Category.ThreadID != nil {
			threadID = *rot.Category.ThreadID
		}

		key := destKey{chatID, threadID, fireAt}
		grouped[key] = append(grouped[key], reminderEntry{
			person:       person,
			categoryName: rot.Category.Name,
		})
	}

	reminders := make([]scheduledReminder, 0, len(grouped))
	for key, entries := range grouped {
		reminders = append(reminders, scheduledReminder{
			fireAt:   key.fireAt,
			chatID:   key.chatID,
			threadID: key.threadID,
			entries:  entries,
		})
	}
	sort.Slice(reminders, func(i, j int) bool {
		return reminders[i].fireAt.Before(reminders[j].fireAt)
	})
	return reminders, nil
}

func (h *Handler) fireReminder(ctx context.Context, b *tgbot.Bot, r scheduledReminder) {
	var sb strings.Builder
	sb.WriteString("🔔 On duty today:\n")
	for _, e := range r.entries {
		sb.WriteString(fmt.Sprintf("• @%s — %s\n", e.person.TelegramUsername, e.categoryName))
	}

	params := &tgbot.SendMessageParams{
		ChatID: r.chatID,
		Text:   strings.TrimRight(sb.String(), "\n"),
	}
	if r.threadID != 0 {
		params.MessageThreadID = r.threadID
	}
	if _, err := b.SendMessage(ctx, params); err != nil {
		log.Printf("⚠️ reminder: failed to send to chatID=%d threadID=%d: %v", r.chatID, r.threadID, err)
		return
	}
	log.Printf("✓ reminder: sent to chatID=%d threadID=%d", r.chatID, r.threadID)
}

// handleReminderOn enables reminders and prints the schedule plan for today and tomorrow.
func (h *Handler) handleReminderOn(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if !isAdmin(h.cfg, msg.From.Username) {
		h.sendMessage(ctx, b, msg, "⚠️ This command is for admins only.")
		return
	}
	if msg.Chat.Type != "private" {
		h.sendMessage(ctx, b, msg, "⚠️ Use /reminder_on in DM.")
		return
	}

	if err := h.storage.SetReminderEnabled(ctx, true); err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to enable reminders: %v", err))
		return
	}

	// Signal the scheduler to recompute immediately (non-blocking)
	select {
	case h.reminderCh <- struct{}{}:
	default:
	}

	h.sendMessage(ctx, b, msg, h.buildReminderPlanText(ctx))
}

// handleReminderOff disables all reminders.
func (h *Handler) handleReminderOff(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if !isAdmin(h.cfg, msg.From.Username) {
		h.sendMessage(ctx, b, msg, "⚠️ This command is for admins only.")
		return
	}
	if msg.Chat.Type != "private" {
		h.sendMessage(ctx, b, msg, "⚠️ Use /reminder_off in DM.")
		return
	}

	if err := h.storage.SetReminderEnabled(ctx, false); err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to disable reminders: %v", err))
		return
	}
	h.sendMessage(ctx, b, msg, "Reminders disabled.")
}

// buildReminderPlanText returns a human-readable plan of upcoming reminders.
func (h *Handler) buildReminderPlanText(ctx context.Context) string {
	now := time.Now()

	todayPlan, _ := h.buildReminderPlan(ctx, now)
	tomorrowPlan, _ := h.buildReminderPlan(ctx, now.Add(24*time.Hour))

	// Filter today to only upcoming reminders
	var upcomingToday []scheduledReminder
	for _, r := range todayPlan {
		if r.fireAt.After(now) {
			upcomingToday = append(upcomingToday, r)
		}
	}

	var sb strings.Builder
	sb.WriteString("✅ Reminders enabled.\n")

	if len(upcomingToday) == 0 && len(tomorrowPlan) == 0 {
		sb.WriteString("\nNo reminders scheduled — no categories with assigned persons and work hours configured.")
		return sb.String()
	}

	if len(upcomingToday) > 0 {
		sb.WriteString("\nToday:\n")
		for _, r := range upcomingToday {
			sb.WriteString(h.formatReminderLine(r))
		}
	}
	if len(tomorrowPlan) > 0 {
		sb.WriteString("\nTomorrow:\n")
		for _, r := range tomorrowPlan {
			sb.WriteString(h.formatReminderLine(r))
		}
	}
	return sb.String()
}

func (h *Handler) formatReminderLine(r scheduledReminder) string {
	// Show local time of the first person (they share the same fireAt)
	var timeStr string
	if len(r.entries) > 0 {
		p := r.entries[0].person
		loc := time.UTC
		if p.Timezone != "" {
			if parsed, err := storage.ParseTimezone(p.Timezone); err == nil {
				loc = parsed
			}
		}
		timeStr = r.fireAt.In(loc).Format("15:04 (UTC-07:00)")
		// Use the actual offset string stored in the person's record for clarity
		if p.Timezone != "" {
			timeStr = r.fireAt.In(loc).Format("15:04") + " (" + p.Timezone + ")"
		}
	}

	var who []string
	for _, e := range r.entries {
		who = append(who, fmt.Sprintf("@%s (%s)", e.person.TelegramUsername, e.categoryName))
	}

	dest := h.formatReminderDest(r.chatID, r.threadID)
	return fmt.Sprintf("  %s — %s → %s\n", timeStr, strings.Join(who, ", "), dest)
}

func (h *Handler) formatReminderDest(chatID int64, threadID int) string {
	groupName := h.getGroupName(chatID)
	if groupName == "" {
		groupName = fmt.Sprintf("chat %d", chatID)
	}
	if threadID == 0 {
		return groupName
	}
	topics := h.getTopics(chatID)
	topicName := topics[threadID]
	if topicName == "" {
		topicName = fmt.Sprintf("topic %d", threadID)
	}
	return groupName + " / " + topicName
}

func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, time.UTC)
}
