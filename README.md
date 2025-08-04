# Discord Bot

A scriptable Discord bot built in Go with Lua scripting support.

## Features

- **Scriptable**: Write bot functionality in Lua
- **Hot Reloading**: Scripts automatically reload when modified
- **Persistent Storage**: SQLite database for persistent data
- **Graceful Shutdown**: Proper cleanup on termination
- **Modular Design**: Clean separation of concerns
- **Error Tracking**: Script-specific error reporting with file names
- **Thread-Safe Lua**: Single-threaded event queue ensures Lua state safety
- **Bot Commands**: Register custom commands with optional cooldowns

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
- `register_command(name, description, callback, cooldown)` - Register a bot command
- `get_commands()` - Get a table of all registered commands
- `store_set(namespace, key, value)` - Store persistent data
- `store_get(namespace, key)` - Retrieve persistent data
- `store_get_all(namespace)` - Retrieve all data from a namespace
- `store_delete(namespace, key)` - Delete persistent data
- `http_get(url, options)` - Perform HTTP GET request
- `http_post(url, body, options)` - Perform HTTP POST request
- `json_encode(table)` - Convert Lua table to JSON string
- `json_decode(string)` - Convert JSON string to Lua table
- `log(message)` - Log a message to the bot's console
- `call_later(seconds, callback, data)` - Register a one-shot timer callback
- `register_timer(seconds, callback, data)` - Register a repeating timer callback
- `unregister_timer(timer_id)` - Cancel a registered timer

### Bot Commands

Commands provide a structured way to handle user interactions. Commands are triggered when users type messages starting with `!` followed by the command name.

#### Command Registration

Use `register_command(name, description, callback, cooldown)` to register a new command:

- `name` (string): The command name (without the `!` prefix)
- `description` (string): A description of what the command does
- `callback` (function): The function to execute when the command is used
- `cooldown` (number, optional): Cooldown period in seconds (default: no cooldown)

#### Command Callback Function

Your callback function receives an event table with:
- `event.args` - Table containing command arguments (index 1 is the command name)
- `event.channel_id` - The Discord channel ID where the command was used
- `event.author` - The username of the person who used the command

#### Example Command

```lua
-- Simple ping command with 10-second cooldown
function handle_ping(event)
    send_message(event.channel_id, "Pong!")
end

register_command("ping", "Replies with Pong!", handle_ping, 10)
```

When a user types `!ping`, the bot will respond with "Pong!" and the command will be unavailable for 10 seconds for all users.

#### Advanced Command Examples

```lua
-- Command with arguments
function handle_echo(event)
    local message = table.concat(event.args, " ", 2) -- Skip the command name
    if message == "" then
        send_message(event.channel_id, "Usage: !echo <message>")
    else
        send_message(event.channel_id, message)
    end
end

register_command("echo", "Echoes back your message", handle_echo, 5)

-- Command that lists all available commands
function handle_help(event)
    local commands = get_commands()
    local helpText = "Available commands:\n"
    
    for name, cmd in pairs(commands) do
        helpText = helpText .. "!" .. name .. " - " .. cmd.description .. "\n"
    end
    
    send_message(event.channel_id, helpText)
end

register_command("help", "Shows all available commands", handle_help, 30)

-- Command with HTTP request
function handle_weather(event)
    local city = table.concat(event.args, " ", 2)
    if city == "" then
        send_message(event.channel_id, "Usage: !weather <city>")
        return
    end
    
    local response = http_get("https://api.example.com/weather?city=" .. city, {
        headers = {["Accept"] = "application/json"},
        timeout = 5
    })
    
    if response and response.status == 200 then
        local data = json_decode(response.body)
        send_message(event.channel_id, "Weather in " .. city .. ": " .. data.temperature .. "°C")
    else
        send_message(event.channel_id, "Failed to get weather data for " .. city)
    end
end

register_command("weather", "Get weather for a city", handle_weather, 60)
```

#### Command Features

- **Unique Registration**: Only one script can register each command name
- **Global Cooldowns**: Cooldowns apply globally to prevent abuse from multiple users
- **Argument Parsing**: Command arguments are automatically parsed and available in `event.args`
- **Thread Safety**: Commands are processed through the same event queue as other Lua events
- **Hot Reloading**: Commands are automatically re-registered when scripts are reloaded

### Hook Types

- `on_channel_message` - Triggered for messages in channels
- `on_direct_message` - Triggered for direct messages
- `on_shutdown` - Triggered when the bot is shutting down gracefully

### Event Queue Architecture

The Lua engine uses a single-threaded event queue system to ensure thread safety:

- **Event Queue**: All Lua execution goes through a buffered channel (`LuaEvent`)
- **Dispatcher**: A dedicated goroutine processes events sequentially
- **State Safety**: Only one Lua function executes at a time on the main `*lua.LState`
- **Graceful Shutdown**: The dispatcher stops cleanly when the bot shuts down
- **Timer System**: Timers are executed through the same event queue for consistency

This architecture prevents race conditions and makes the system more predictable and debuggable.

### Example Scripts

```lua
-- Simple echo bot
register_hook("on_channel_message", function(event)
    if event.content == "!ping" then
        send_message(event.channel_id, "Pong!")
    end
end)

-- HTTP API example
register_hook("on_channel_message", function(event)
    if event.content == "!weather" then
        local response = http_get("https://api.example.com/weather", {
            headers = {["Accept"] = "application/json"},
            timeout = 5
        })
        if response and response.status == 200 then
            send_message(event.channel_id, "Weather: " .. response.body)
        end
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