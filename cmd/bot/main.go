package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/mr-exz/cibot/internal/linear"
	"github.com/mr-exz/cibot/internal/telegram"
)

func main() {
	log.Println("cibot starting...")

	ctx := context.Background()

	// Initialize Linear client
	linearClient, err := linear.New(ctx)
	if err != nil {
		log.Fatalf("failed to create Linear client: %v", err)
	}

	// Initialize Telegram bot
	bot, err := telegram.New(ctx, linearClient)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	// Log allowed chat IDs
	allowedChatStr := os.Getenv("ALLOWED_CHAT_ID")
	if allowedChatStr == "" {
		log.Println("⚠️  ALLOWED_CHAT_ID not set - bot will respond to all chats")
	} else {
		chatIDs := strings.Split(allowedChatStr, ",")
		log.Printf("✓ cibot configured for %d chat(s): %s\n", len(chatIDs), allowedChatStr)
	}

	log.Println("cibot running...")
	bot.Start(ctx)
}
