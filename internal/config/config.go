package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramToken  string
	LinearAPIKey   string
	AllowedChatIDs []int64
	AdminUsernames map[string]bool // telegram username -> true (username without @)
	DBPath         string
}

func Load() *Config {
	cfg := &Config{
		TelegramToken:  os.Getenv("TELEGRAM_TOKEN"),
		LinearAPIKey:   os.Getenv("LINEAR_API_KEY"),
		DBPath:         os.Getenv("DB_PATH"),
		AdminUsernames: make(map[string]bool),
	}

	// Validate required env vars
	if cfg.TelegramToken == "" {
		log.Fatalf("TELEGRAM_TOKEN env var is required")
	}
	if cfg.LinearAPIKey == "" {
		log.Fatalf("LINEAR_API_KEY env var is required")
	}

	// Default DB path
	if cfg.DBPath == "" {
		cfg.DBPath = "cibot.db"
	}

	// Parse ALLOWED_CHAT_ID (comma-separated int64 values)
	allowedChatStr := os.Getenv("ALLOWED_CHAT_ID")
	if allowedChatStr != "" {
		chatIDs := strings.Split(allowedChatStr, ",")
		for _, idStr := range chatIDs {
			idStr = strings.TrimSpace(idStr)
			chatID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				log.Printf("⚠️  Invalid chat ID in ALLOWED_CHAT_ID: %s (skipped)", idStr)
				continue
			}
			cfg.AllowedChatIDs = append(cfg.AllowedChatIDs, chatID)
		}
		if len(cfg.AllowedChatIDs) == 0 {
			log.Println("⚠️  ALLOWED_CHAT_ID not set or contains no valid IDs - bot will respond to all chats")
		} else {
			log.Printf("✓ Allowed chats: %d configured", len(cfg.AllowedChatIDs))
		}
	} else {
		log.Println("⚠️  ALLOWED_CHAT_ID not set - bot will respond to all chats")
	}

	// Parse ADMIN_USERNAMES (comma-separated Telegram usernames, with or without @)
	adminUserStr := os.Getenv("ADMIN_USERNAMES")
	if adminUserStr != "" {
		usernames := strings.Split(adminUserStr, ",")
		for _, username := range usernames {
			username = strings.TrimSpace(username)
			// Remove @ prefix if present
			username = strings.TrimPrefix(username, "@")
			if username == "" {
				log.Printf("⚠️  Invalid admin username (skipped)")
				continue
			}
			cfg.AdminUsernames[username] = true
		}
		log.Printf("✓ Admin usernames configured: %d", len(cfg.AdminUsernames))
	}

	return cfg
}

// IsAllowedChat checks if the chat ID is in the allowed list
// Returns true if no chat restrictions are configured (permissive default)
func (c *Config) IsAllowedChat(chatID int64) bool {
	if len(c.AllowedChatIDs) == 0 {
		return true
	}
	for _, id := range c.AllowedChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

// IsPrivateChatAllowed checks if a private chat should be allowed
// Allows: admins in private chats, configured group chats
func (c *Config) IsPrivateChatAllowed(chatID int64, chatType string, username string) bool {
	// Allow configured group chats
	if chatType != "private" {
		return c.IsAllowedChat(chatID)
	}
	// Allow private chats for admins only
	return c.IsAdmin(username)
}

// IsAdmin checks if the username is an admin
// username should be without @ prefix
func (c *Config) IsAdmin(username string) bool {
	if username == "" {
		return false
	}
	return c.AdminUsernames[username]
}
