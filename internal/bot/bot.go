package bot

import (
	"context"
	"log"

	"github.com/bwmarrin/discordgo"

	"github.com/leihog/discord-bot/internal/config"
	"github.com/leihog/discord-bot/internal/database"
	"github.com/leihog/discord-bot/internal/lua"
	"github.com/leihog/discord-bot/internal/users"
)

// Bot represents the Discord bot
type Bot struct {
	session   *discordgo.Session
	db        *database.DB
	engine    *lua.Engine
	watcher   *lua.Watcher
	config    *config.Config
	userStore *users.Store
}

// New creates a new bot instance
func New(cfg *config.Config) (*Bot, error) {
	// Create Discord session
	session, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, err
	}

	// Initialize database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	if err := db.Initialize(); err != nil {
		return nil, err
	}

	userStore := users.New(db)

	// Create Lua engine
	engine := lua.New(db, session, userStore)
	engine.Initialize()

	// Create file watcher
	watcher := lua.NewWatcher(engine, cfg.ScriptsDir)

	return &Bot{
		session:   session,
		db:        db,
		engine:    engine,
		watcher:   watcher,
		config:    cfg,
		userStore: userStore,
	}, nil
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) error {
	// Set up Discord intents
	b.session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds | discordgo.IntentsDirectMessages

	// Add message handler
	b.session.AddHandler(b.onMessageCreate) // todo this should be done after LuaEngine is started

	// Open Discord connection
	if err := b.session.Open(); err != nil {
		return err
	}

	// Bootstrap admin if no admin exists yet
	if err := b.userStore.Bootstrap(); err != nil {
		log.Println("Warning: admin bootstrap failed:", err)
	}

	// Load initial scripts
	b.engine.LoadScripts(b.config.ScriptsDir) // todo: this could be done in Initialize or Start

	// Start Lua engine dispatcher
	b.engine.Start(ctx)

	// Start file watcher
	b.watcher.Start(ctx)

	log.Println("Bot is now running. Press CTRL+C to exit.")
	return nil
}

// Stop gracefully shuts down the bot
func (b *Bot) Stop() error {
	log.Println("Received shutdown signal. Gracefully shutting down...")

	// Close Lua engine
	b.engine.Close()

	// Close Discord session
	if err := b.session.Close(); err != nil {
		log.Println("Error closing Discord session:", err)
	}

	// Close database
	if err := b.db.Close(); err != nil {
		log.Println("Error closing database:", err)
	}

	log.Println("Bot shutdown complete.")
	return nil
}

// onMessageCreate handles Discord message events
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	b.engine.ProcessMessage(m)
}
