package main

import (
	"log"

	"github.com/leihog/discord-bot/internal/bot"
	"github.com/leihog/discord-bot/internal/config"
	"github.com/leihog/discord-bot/internal/utils"
)

func main() {
	// Load configuration
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal("Configuration error:", err)
	}

	// Set up graceful shutdown
	ctx, cancel := utils.SetupGracefulShutdown()
	defer cancel()

	// Create bot instance
	b, err := bot.New(cfg)
	if err != nil {
		log.Fatal("Failed to create bot:", err)
	}

	// Start the bot
	if err := b.Start(ctx); err != nil {
		log.Fatal("Failed to start bot:", err)
	}

	// Wait for shutdown signal
	<-ctx.Done()

	// Stop the bot gracefully
	if err := b.Stop(); err != nil {
		log.Fatal("Failed to stop bot:", err)
	}
}
