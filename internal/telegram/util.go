package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/config"
	"github.com/mr-exz/cibot/internal/storage"
)

// parseLocation resolves a timezone string to a *time.Location.
// Accepts IANA names ("UTC", "Asia/Almaty") and fixed-offset strings ("+05:00", "-07:30").
func parseLocation(tz string) (*time.Location, error) {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc, nil
	}
	return storage.ParseTimezone(tz)
}

// ButtonItem represents a button with label and callback data
type ButtonItem struct {
	Label string
	ID    int64
}

// buildKeyboard creates an inline keyboard with items, 2 per row
func buildKeyboard(items []ButtonItem, prefix string) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0)
	for i := 0; i < len(items); i += 2 {
		row := []models.InlineKeyboardButton{
			{
				Text:         items[i].Label,
				CallbackData: fmt.Sprintf("%s%d", prefix, items[i].ID),
			},
		}
		if i+1 < len(items) {
			row = append(row, models.InlineKeyboardButton{
				Text:         items[i+1].Label,
				CallbackData: fmt.Sprintf("%s%d", prefix, items[i+1].ID),
			})
		}
		rows = append(rows, row)
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "❌ Cancel", CallbackData: "cancel"},
	})

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// buildCategoryKeyboard creates a keyboard from category list
func buildCategoryKeyboard(categories []storage.Category) *models.InlineKeyboardMarkup {
	items := make([]ButtonItem, len(categories))
	for i, cat := range categories {
		items[i] = ButtonItem{
			Label: cat.Emoji + " " + cat.Name,
			ID:    cat.ID,
		}
	}
	return buildKeyboard(items, "cat:")
}

// buildPriorityKeyboard creates the hardcoded priority selection keyboard.
// Linear priority values: 1=Urgent, 2=High, 3=Medium, 4=Low
func buildPriorityKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🔴 P0 — now", CallbackData: "prio:1"}, {Text: "🟠 P1 — today", CallbackData: "prio:2"}},
			{{Text: "🟡 P2 — week", CallbackData: "prio:3"}, {Text: "🔵 P3 — later", CallbackData: "prio:4"}},
			{{Text: "❌ Cancel", CallbackData: "cancel"}},
		},
	}
}

// priorityLabel returns the display label for a Linear priority value.
func priorityLabel(p int) string {
	switch p {
	case 1:
		return "🔴 P0 — now"
	case 2:
		return "🟠 P1 — today"
	case 3:
		return "🟡 P2 — week"
	case 4:
		return "🔵 P3 — later"
	default:
		return "—"
	}
}

// buildRequestTypeKeyboard creates a keyboard from request type list
func buildRequestTypeKeyboard(types []storage.RequestType) *models.InlineKeyboardMarkup {
	items := make([]ButtonItem, len(types))
	for i, t := range types {
		items[i] = ButtonItem{
			Label: t.Name,
			ID:    t.ID,
		}
	}
	return buildKeyboard(items, "type:")
}

// parseTelegramLink parses a Telegram message link and extracts chat ID and message ID.
// Supports formats:
// - https://t.me/c/CHATID/MSGID          (private group)
// - https://t.me/c/CHATID/THREADID/MSGID (threaded message in a topic)
func parseTelegramLink(link string) (chatID int64, messageID int, err error) {
	// Match private group, with optional thread segment: /c/CHATID[/THREADID]/MSGID
	privateRe := regexp.MustCompile(`https://t\.me/c/(\d+)/(?:\d+/)?(\d+)`)
	if matches := privateRe.FindStringSubmatch(link); len(matches) > 0 {
		numericID, _ := strconv.ParseInt(matches[1], 10, 64)
		chatID = -(1_000_000_000_000 + numericID)
		messageID, _ = strconv.Atoi(matches[2])
		return chatID, messageID, nil
	}

	// Try public channel/group format: https://t.me/channel_name/123
	publicRe := regexp.MustCompile(`https://t\.me/([^/]+)/(\d+)`)
	if matches := publicRe.FindStringSubmatch(link); len(matches) > 0 {
		// For public channels/groups we can't determine chat ID from link alone
		// Return error - these would need special handling
		return 0, 0, fmt.Errorf("public channel links not yet supported; use private group link format")
	}

	return 0, 0, fmt.Errorf("invalid Telegram message link format")
}

// formatTelegramLink creates a Telegram message link from chat ID, optional thread ID, and message ID.
// Pass threadID=0 when not in a topic thread.
func formatTelegramLink(chatID int64, threadID int, messageID int) string {
	if chatID < 0 {
		// supergroup/channel: chatID = -100XXXXXXXXXX
		numericID := (-chatID) - 1_000_000_000_000
		if threadID != 0 {
			return fmt.Sprintf("https://t.me/c/%d/%d/%d", numericID, threadID, messageID)
		}
		return fmt.Sprintf("https://t.me/c/%d/%d", numericID, messageID)
	}
	// For public chats we can't create a link from numeric ID alone
	return ""
}

// formatMediaLinks converts a list of media URLs to markdown format
func formatMediaLinks(mediaLinks []string) string {
	if len(mediaLinks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n**Media:**")
	for i, link := range mediaLinks {
		sb.WriteString(fmt.Sprintf("\n%d. [Attachment](%s)", i+1, link))
	}
	return sb.String()
}

// setChatMemberTag calls the Bot API 9.5 setChatMemberTag method directly.
// tag can be empty to remove the tag.
func setChatMemberTag(ctx context.Context, token string, chatID int64, userID int64, tag string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"user_id": userID,
		"tag":     tag,
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setChatMemberTag", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram: %s", result.Description)
	}
	return nil
}

// isAdmin checks if a user is an admin
func isAdmin(cfg *config.Config, username string) bool {
	return cfg.IsAdmin(username)
}

// buildTypeSelectKeyboard creates a keyboard with existing types + a "New type" option
func buildTypeSelectKeyboard(types []storage.RequestType) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(types)+2)
	for _, t := range types {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         t.Name,
			CallbackData: fmt.Sprintf("type_sel:%d", t.ID),
		}})
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: "✏️ New type", CallbackData: "type_sel:new"}},
		[]models.InlineKeyboardButton{{Text: "❌ Cancel", CallbackData: "cancel"}},
	)
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildPersonKeyboard creates a keyboard from support person list
func buildPersonKeyboard(persons []storage.SupportPerson) *models.InlineKeyboardMarkup {
	items := make([]ButtonItem, len(persons))
	for i, person := range persons {
		items[i] = ButtonItem{
			Label: person.Name + " (@" + person.TelegramUsername + ")",
			ID:    person.ID,
		}
	}
	return buildKeyboard(items, "person:")
}

// buildRotationTypeKeyboard creates a keyboard for rotation type selection
func buildRotationTypeKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "📅 Daily",
					CallbackData: "rot:daily",
				},
				{
					Text:         "📊 Weekly",
					CallbackData: "rot:weekly",
				},
			},
		},
	}
}

// buildPickerKeyboard builds a two-column inline keyboard from a list of string values.
// Each button's label and callback data are the value itself, prefixed by callbackPrefix.
// An optional Skip button is appended when canSkip is true.
func buildPickerKeyboard(values []string, callbackPrefix string, canSkip bool) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(values)/2+2)
	for i := 0; i < len(values); i += 2 {
		row := []models.InlineKeyboardButton{{
			Text:         values[i],
			CallbackData: callbackPrefix + values[i],
		}}
		if i+1 < len(values) {
			row = append(row, models.InlineKeyboardButton{
				Text:         values[i+1],
				CallbackData: callbackPrefix + values[i+1],
			})
		}
		rows = append(rows, row)
	}
	if canSkip {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text: "⏭ Skip", CallbackData: callbackPrefix + "skip",
		}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildSkipKeyboard creates a keyboard with a single Skip button
func buildSkipKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "⏭ Skip",
					CallbackData: "skip",
				},
			},
		},
	}
}

// buildTopicConfirmKeyboard creates a keyboard for topic linking confirmation
func buildTopicConfirmKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "✓ Link to this topic",
					CallbackData: "confirm:topic",
				},
				{
					Text:         "🌐 Make global",
					CallbackData: "confirm:global",
				},
			},
			{
				{
					Text:         "❌ Cancel",
					CallbackData: "cancel",
				},
			},
		},
	}
}

// topicEntry is a sorted topic map entry.
type topicEntry struct {
	ThreadID int
	Name     string
}

// sortTopics converts a threadID→name map into a slice sorted by name.
func sortTopics(topics map[int]string) []topicEntry {
	entries := make([]topicEntry, 0, len(topics))
	for threadID, name := range topics {
		entries = append(entries, topicEntry{threadID, name})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// buildGroupKeyboard creates a keyboard for group selection, sorted by group name.
func buildGroupKeyboard(groups map[int64]string) *models.InlineKeyboardMarkup {
	type groupEntry struct {
		chatID int64
		name   string
	}
	entries := make([]groupEntry, 0, len(groups))
	for chatID, name := range groups {
		entries = append(entries, groupEntry{chatID, name})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	rows := make([][]models.InlineKeyboardButton, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         e.name,
			CallbackData: fmt.Sprintf("grp:%d", e.chatID),
		}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildTopicSelectionKeyboard creates a keyboard for topic selection (DM only)
func buildTopicSelectionKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "🌐 Make global (all topics)",
					CallbackData: "confirm:global",
				},
			},
			{
				{
					Text:         "📝 Enter topic ID manually",
					CallbackData: "topic:manual",
				},
			},
		},
	}
}
