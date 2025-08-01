# Discord Bot

A scriptable Discord bot built in Go with Lua scripting support.

## Features

- **Scriptable**: Write bot functionality in Lua
- **Hot Reloading**: Scripts automatically reload when modified
- **Persistent Storage**: SQLite database for persistent data
- **Graceful Shutdown**: Proper cleanup on termination
- **Modular Design**: Clean separation of concerns

## Project Structure

```
discord-bot/
├── cmd/
│   └── bot/
│       └── main.go          # Entry point
├── internal/
│   ├── bot/                 # Bot lifecycle management
│   ├── config/              # Configuration management
│   ├── database/            # Database connection
│   └── lua/                 # Lua scripting engine
│   └── utils/               # misc utility functions
├── lua/
│   └── scripts/             # Lua scripts
└── go.mod
```

## Building

```bash
go build -o discord-bot cmd/bot/main.go
```

## Running

1. Set the `DISCORD_BOT_TOKEN` environment variable
2. Run the bot:
   ```bash
   ./discord-bot
   ```

## Lua Scripting

### Available Functions

- `send_message(channel_id, message)` - Send a message to a channel
- `register_hook(hook_name, function)` - Register event handlers
- `store_set(namespace, key, value)` - Store persistent data
- `store_get(namespace, key)` - Retrieve persistent data
- `store_get_all(namespace)` - Retrieve all data from a namespace
- `store_delete(namespace, key)` - Delete persistent data

### Hook Types

- `on_channel_message` - Triggered for messages in channels
- `on_direct_message` - Triggered for direct messages

### Example Script

```lua
-- Simple echo bot
register_hook("on_channel_message", function(event)
    if event.content == "!ping" then
        send_message(event.channel_id, "Pong!")
    end
end)
```

## Configuration

The bot uses environment variables for configuration:

- `DISCORD_BOT_TOKEN` - Discord bot token (required)

## Development

### Adding New Lua Functions

1. Add the function to `internal/lua/functions.go`
2. Register it in the `registerFunctions()` method
3. The function will be available to all Lua scripts

### Adding New Bot Features

1. Create a new module in `internal/` if needed
2. Add the feature to the appropriate existing module
3. Update the bot initialization in `internal/bot/bot.go` if necessary 