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
- **Bot Commands**: Register custom commands with optional cooldowns and role requirements
- **User Management**: Automatic user tracking with bot-side roles and extensible per-user metadata

## Project Structure

```
discord-bot/
├── cmd/
│   ├── bot/
│   │   └── main.go          # Bot entry point
│   └── dev/
│       └── main.go          # Dev shell entry point
├── internal/
│   ├── bot/                 # Bot lifecycle management
│   ├── config/              # Configuration management
│   ├── database/            # Database connection
│   ├── lua/                 # Lua scripting engine
│   ├── users/               # User management (roles, metadata)
│   └── utils/               # misc utility functions
├── scripts/                 # Lua scripts
└── go.mod
```

## Building

```bash
# Production bot
go build -o discord-bot ./cmd/bot

# Dev shell
go build -o discord-dev ./cmd/dev
```

## Running

1. Set the `DISCORD_BOT_TOKEN` environment variable
2. Run the bot:
   ```bash
   ./discord-bot
   ```

## Dev Shell

The dev shell lets you interact with the Lua scripting engine locally without a real Discord connection. It provides a terminal TUI with a scrollable output viewport and an interactive prompt.

```bash
./discord-dev --scripts-dir scripts --db :memory:
```

```
┌──────────────────────────────────┐
│  Script loaded: jokes.lua        │
│  [Bot → #dev-channel]: Pong!     │
│  ...                             │
├──────────────────────────────────┤
│  [#dev-channel] dev> █           │
└──────────────────────────────────┘
```

Bot replies and log output appear in the viewport asynchronously without corrupting the input prompt. Mouse-wheel scrolling is supported.

### Dev shell commands

| Command | Description |
|---|---|
| `/channel <id>` | Switch active channel ID |
| `/user <name> [id]` | Change the simulated author name and optional ID |
| `/dm` | Toggle DM mode (`on_direct_message` vs `on_channel_message`) |
| `/scripts` | List loaded scripts |
| `/reload <name>` | Reload a script by name (e.g. `jokes.lua`) |
| `/commands` | List registered bot commands |
| `/hooks` | List registered hooks and which scripts own them |
| `/lua <code>` | Execute a Lua snippet and print the result |
| `/quit`, `/exit` | Exit the dev shell |
| `Ctrl+C` | Exit the dev shell |

Any input not starting with `/` is dispatched as a message from the simulated user, triggering hooks and commands exactly as they would fire on Discord.

## Lua Scripting

### Available Functions

**Messaging**
- `send_message(channel_id, message)` - Send a message to a channel

**Commands & Hooks**
- `register_hook(hook_name, function)` - Register event handlers
- `register_command(name, description, callback[, cooldown[, required_role]])` - Register a bot command
- `get_commands()` - Get a table of all registered commands

**Persistent Storage**
- `store_set(namespace, key, value)` - Store persistent data
- `store_get(namespace, key)` - Retrieve persistent data
- `store_get_all(namespace)` - Retrieve all data from a namespace
- `store_delete(namespace, key)` - Delete persistent data

**User Management**
- `user_ensure(id, display_name)` - Upsert a user record
- `user_get(id)` - Get user info: `{id, display_name, roles, created_at}` or nil
- `user_has_role(id, role)` - Check if a user has a role (returns bool)
- `user_add_role(id, role)` - Grant a role to a user
- `user_remove_role(id, role)` - Revoke a role from a user
- `user_set_meta(id, key, value)` - Store arbitrary metadata for a user
- `user_get_meta(id, key)` - Retrieve a metadata value (returns string or nil)
- `user_get_all_meta(id)` - Get all metadata for a user as a table

**HTTP**
- `http_get(url, options)` - Perform HTTP GET request
- `http_post(url, body, options)` - Perform HTTP POST request

**JSON**
- `json_encode(table)` - Convert Lua table to JSON string
- `json_decode(string)` - Convert JSON string to Lua table

**Timers**
- `call_later(seconds, callback, data)` - Register a one-shot timer callback
- `register_timer(seconds, callback, data)` - Register a repeating timer callback
- `unregister_timer(timer_id)` - Cancel a registered timer

**Utilities**
- `log(message)` - Log a message to the bot's console

### Bot Commands

Commands provide a structured way to handle user interactions. Commands are triggered when users type messages starting with `!` followed by the command name.

#### Command Registration

Use `register_command(name, description, callback[, cooldown[, required_role]])` to register a new command:

- `name` (string): The command name (without the `!` prefix)
- `description` (string): A description of what the command does
- `callback` (function): The function to execute when the command is used
- `cooldown` (number, optional): Cooldown period in seconds (default: no cooldown)
- `required_role` (string, optional): Role the caller must have; bot replies "Permission denied." otherwise

#### Command Callback Function

Your callback function receives an event table with:
- `event.args` - Table containing command arguments (index 1 is the command name)
- `event.channel_id` - The Discord channel ID where the command was used
- `event.author` - The username of the person who used the command
- `event.author_id` - The ID of the person who triggered the command

#### Example Command

```lua
-- Simple ping command with 10-second cooldown
function handle_ping(event)
    send_message(event.channel_id, "Pong!")
end

register_command("ping", "Replies with Pong!", handle_ping, 10)

-- Admin-only command (no cooldown needed, so pass 0)
register_command("kick", "Kick a user", function(event)
    send_message(event.channel_id, "Kicking " .. (event.args[2] or "nobody"))
end, 0, "admin")
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

### User Management

The bot automatically tracks every Discord user it sees. No registration is required — a user record is created the first time a message from that user is processed. New users are assigned the `user` role automatically.

#### Roles

Two built-in roles exist: `user` (all members) and `admin`. Roles are bot-side only and independent of Discord server roles.

**Admin bootstrap:** On first start with no admin set, the bot generates a one-time claim token and prints it to the log:

```
[ADMIN BOOTSTRAP] No admin set. DM the bot: !claim_admin XXXXXXXX
```

Send that command to the bot in a DM to claim admin. The token is consumed on use and is not re-generated once an admin exists.

#### Sharing user data between scripts

Scripts can store and read arbitrary per-user metadata using `user_set_meta`/`user_get_meta`. Because these write to the database, the data is available to every script without any coupling between them.

```lua
-- In script A: record when a user last used a feature
register_hook("on_channel_message", function(event)
    user_set_meta(event.author_id, "last_seen", tostring(os.time()))
end)

-- In script B: read the value written by script A
register_command("profile", "Show user profile", function(event)
    local last = user_get_meta(event.author_id, "last_seen")
    local msg = last and ("Last seen: " .. last) or "No activity recorded."
    send_message(event.channel_id, msg)
end)
```

#### Example: role-gated admin panel

```lua
register_command("admininfo", "Admin-only info", function(event)
    local u = user_get(event.author_id)
    send_message(event.channel_id, "Roles: " .. table.concat(u.roles, ", "))
end, 0, "admin")
```

### Bot events (hooks)

Outside of getting triggered by commands, scripts can also trigger on various Bot events

- `on_channel_message` - Triggered for messages in channels
- `on_direct_message` - Triggered for direct messages
- `on_shutdown` - Triggered when the bot is shutting down gracefully
- `on_unload`- Triggered when the script is unloaded


#### Example Script

```lua
register_hook("on_channel_message", function(event)
    if event.content == "ping" then
        send_message(event.channel_id, "Pong!")
    end
end)
```

#### Event data

A registered hook callback function receives an event table with:
- `event.content` - A string containing a recieved discord message
- `event.channel_id` - The Discord channel ID where the event took place
- `event.author` - The username of the person who triggered the event
- `event.author_id` - The ID of the person who triggered the event

### Notes and considerations

- On bot shutdown, all queued timers are cleared without firing.
- Trying to register new timers during shutdown or while the active script is unloading will result in error. 

## Configuration

The bot is configured via environment variables:

| Variable | Required | Default | Description |
|---|---|---|---|
| `DISCORD_BOT_TOKEN` | Yes | — | Discord bot token |
| `SCRIPTS_DIR` | No | `scripts` | Directory containing Lua scripts |
| `DATABASE_PATH` | No | `data/bot.db` | SQLite database path |

## Development

### Adding New Lua Functions

1. Add the function to `internal/lua/functions.go`
2. Register it in the `registerFunctions()` method
3. The function will be available to all Lua scripts

### Adding New Bot Features

1. Create a new module in `internal/` if needed
2. Add the feature to the appropriate existing module
3. Update the bot initialization in `internal/bot/bot.go` if necessary


