package config

import (
	"log"
	"os"
	"strings"
)

type Config struct {
	TelegramToken  string
	LinearAPIKey   string
	AdminUsernames map[string]bool // telegram username -> true (username without @)
	DBPath         string
	CSVPath        string
	WebPort        string
	WebDomain      string // if set, enables Let's Encrypt TLS (listens on :443 + :80 redirect)
	WebCertDir     string // directory to cache Let's Encrypt certificates
}

func Load() *Config {
	cfg := &Config{
		TelegramToken:  os.Getenv("TELEGRAM_TOKEN"),
		LinearAPIKey:   os.Getenv("LINEAR_API_KEY"),
		DBPath:         os.Getenv("DB_PATH"),
		CSVPath:        os.Getenv("MESSAGES_CSV"),
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

	// Default CSV path
	if cfg.CSVPath == "" {
		cfg.CSVPath = "messages.csv"
	}

	cfg.WebPort = os.Getenv("WEB_PORT")
	if cfg.WebPort == "" {
		cfg.WebPort = "8000"
	}

	cfg.WebDomain = os.Getenv("WEB_DOMAIN")

	cfg.WebCertDir = os.Getenv("WEB_CERT_DIR")
	if cfg.WebCertDir == "" {
		cfg.WebCertDir = "/data/autocert"
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

// IsAdmin checks if the username is an admin
// username should be without @ prefix
func (c *Config) IsAdmin(username string) bool {
	if username == "" {
		return false
	}
	return c.AdminUsernames[username]
}
