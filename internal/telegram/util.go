package telegram

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/config"
	"github.com/mr-exz/cibot/internal/storage"
)

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

// formatTelegramLink creates a Telegram message link from chat ID and message ID
func formatTelegramLink(chatID int64, messageID int) string {
	if chatID < 0 {
		// supergroup/channel: chatID = -100XXXXXXXXXX
		// Extract numeric part: -(1000000000000 + X) = chatID, so X = -chatID - 1000000000000
		numericID := (-chatID) - 1_000_000_000_000
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

// isAdmin checks if a user is an admin
func isAdmin(cfg *config.Config, username string) bool {
	return cfg.IsAdmin(username)
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
		},
	}
}

// buildGroupKeyboard creates a keyboard for group selection in the /addtopic flow
func buildGroupKeyboard(groups map[int64]string) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(groups))
	for chatID, name := range groups {
		rows = append(rows, []models.InlineKeyboardButton{
			{
				Text:         name,
				CallbackData: fmt.Sprintf("grp:%d", chatID),
			},
		})
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
