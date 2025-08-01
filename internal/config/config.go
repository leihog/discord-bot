package config

import (
	"os"
)

// Config holds all configuration for the bot
type Config struct {
	BotToken     string
	ScriptsDir   string
	DatabasePath string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		BotToken:     os.Getenv("DISCORD_BOT_TOKEN"),
		ScriptsDir:   "lua/scripts",
		DatabasePath: "bot_data.db",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.BotToken == "" {
		return &ConfigError{Field: "DISCORD_BOT_TOKEN", Message: "Bot token is required"}
	}
	return nil
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}
