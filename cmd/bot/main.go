package main

import (
	"context"
	"log"

	"github.com/mr-exz/cibot/internal/config"
	"github.com/mr-exz/cibot/internal/linear"
	"github.com/mr-exz/cibot/internal/storage"
	"github.com/mr-exz/cibot/internal/telegram"
)

var version = "dev"

func main() {
	log.Println("cibot starting...")

	ctx := context.Background()

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := storage.New(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Linear client
	linearClient, err := linear.New(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create Linear client: %v", err)
	}

	// Initialize Telegram bot
	bot, err := telegram.New(ctx, linearClient, db, cfg, version)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	log.Println("cibot running...")
	bot.Start(ctx)
}
