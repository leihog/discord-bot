// main.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/fsnotify/fsnotify"
	lua "github.com/yuin/gopher-lua"
	_ "modernc.org/sqlite"
)

var (
	session               *discordgo.Session
	luaState              *lua.LState
	hookMutex             sync.Mutex
	onChannelMessageHooks []lua.LValue
	onDirectMessageHooks  []lua.LValue
	shutdownChan          chan os.Signal
	db                    *sql.DB
)

func main() {
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}

	// Set up signal handling for graceful shutdown
	shutdownChan = make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	session, err = discordgo.New("Bot " + botToken)
	if err != nil {
		log.Fatal("Failed to create Discord session:", err)
	}

	session.AddHandler(onMessageCreate)
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds | discordgo.IntentsDirectMessages

	err = session.Open()
	if err != nil {
		log.Fatal("Failed to open connection:", err)
	}

	// Initialize SQLite database
	db, err = sql.Open("sqlite", "bot_data.db")
	if err != nil {
		log.Fatal("Failed to open SQLite DB:", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT,
		PRIMARY KEY (namespace, key)
	)`)
	if err != nil {
		log.Fatal("Failed to create kv_store table:", err)
	}

	luaState = lua.NewState()
	registerLuaFunctions(luaState)
	loadLuaScripts("lua/scripts")
	watchLuaScripts("lua/scripts", ctx)

	log.Println("Bot is now running. Press CTRL+C to exit.")

	// Wait for shutdown signal
	<-shutdownChan
	log.Println("Received shutdown signal. Gracefully shutting down...")

	// Cancel context to stop file watcher
	cancel()

	// Close Discord session gracefully
	if err := session.Close(); err != nil {
		log.Println("Error closing Discord session:", err)
	}

	// Close Lua state
	luaState.Close()

	log.Println("Bot shutdown complete.")
}

func registerLuaFunctions(L *lua.LState) {
	L.SetGlobal("send_message", L.NewFunction(func(L *lua.LState) int {
		channelID := L.CheckString(1)
		message := L.CheckString(2)
		_, err := session.ChannelMessageSend(channelID, message)
		if err != nil {
			log.Println("send_message error:", err)
		}
		return 0
	}))

	L.SetGlobal("register_hook", L.NewFunction(func(L *lua.LState) int {
		hookName := L.CheckString(1)
		hookFunc := L.CheckFunction(2)
		hookMutex.Lock()
		defer hookMutex.Unlock()
		switch hookName {
		case "on_channel_message":
			onChannelMessageHooks = append(onChannelMessageHooks, hookFunc)
		case "on_direct_message":
			onDirectMessageHooks = append(onDirectMessageHooks, hookFunc)
		default:
			log.Println("Unknown hook name:", hookName)
		}
		return 0
	}))

	L.SetGlobal("store_set", L.NewFunction(luaStoreSet))
	L.SetGlobal("store_get", L.NewFunction(luaStoreGet))
	L.SetGlobal("store_delete", L.NewFunction(luaStoreDelete))
}

func luaStoreSet(L *lua.LState) int {
	namespace := L.CheckString(1)
	key := L.CheckString(2)
	value := L.CheckAny(3)

	var valStr string
	if tbl, ok := value.(*lua.LTable); ok {
		// Convert Lua table to Go map
		goVal := luaTableToMap(tbl)
		jsonBytes, err := json.Marshal(goVal)
		if err != nil {
			log.Println("Failed to serialize table:", err)
			return 0
		}
		valStr = string(jsonBytes)
	} else {
		valStr = value.String()
	}

	_, err := db.Exec(`INSERT INTO kv_store(namespace, key, value) VALUES (?, ?, ?) 
		ON CONFLICT(namespace, key) DO UPDATE SET value=excluded.value`, namespace, key, valStr)
	if err != nil {
		log.Println("store_set error:", err)
	}
	return 0
}

func luaStoreGet(L *lua.LState) int {
	namespace := L.CheckString(1)
	key := L.CheckString(2)

	row := db.QueryRow(`SELECT value FROM kv_store WHERE namespace = ? AND key = ?`, namespace, key)
	var valStr string
	err := row.Scan(&valStr)
	if err == sql.ErrNoRows {
		L.Push(lua.LNil)
		return 1
	} else if err != nil {
		log.Println("store_get error:", err)
		L.Push(lua.LNil)
		return 1
	}

	// Try to decode as JSON object
	var decoded interface{}
	if json.Unmarshal([]byte(valStr), &decoded) == nil {
		L.Push(goValueToLua(L, decoded))
	} else {
		L.Push(lua.LString(valStr))
	}
	return 1
}

func luaStoreDelete(L *lua.LState) int {
	namespace := L.CheckString(1)
	key := L.CheckString(2)

	_, err := db.Exec(`DELETE FROM kv_store WHERE namespace = ? AND key = ?`, namespace, key)
	if err != nil {
		log.Println("store_delete error:", err)
	}
	return 0
}

func luaTableToMap(tbl *lua.LTable) map[string]interface{} {
	result := make(map[string]interface{})
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		k := key.String()
		switch v := value.(type) {
		case *lua.LTable:
			result[k] = luaTableToMap(v)
		default:
			result[k] = v.String()
		}
	})
	return result
}

func goValueToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, v2 := range val {
			tbl.RawSetString(k, goValueToLua(L, v2))
		}
		return tbl
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case nil:
		return lua.LNil
	default:
		return lua.LString("unsupported")
	}
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}
	data := luaState.NewTable()
	data.RawSetString("content", lua.LString(m.Content))
	data.RawSetString("channel_id", lua.LString(m.ChannelID))
	data.RawSetString("author", lua.LString(m.Author.Username))

	hookMutex.Lock()
	defer hookMutex.Unlock()
	var hooks []lua.LValue
	if m.GuildID == "" {
		hooks = onDirectMessageHooks
	} else {
		hooks = onChannelMessageHooks
	}

	for _, fn := range hooks {
		if err := luaState.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, data); err != nil {
			log.Println("Lua hook error:", err)
		}
	}
}

func loadLuaScripts(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Println("Failed to read script directory:", err)
		return
	}

	onChannelMessageHooks = nil
	onDirectMessageHooks = nil

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".lua" {
			scriptPath := filepath.Join(dir, f.Name())
			if err := luaState.DoFile(scriptPath); err != nil {
				log.Println("Failed to load script", f.Name(), ":", err)
			}
		}
	}
}

func watchLuaScripts(dir string, ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("File watcher error:", err)
		return
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if filepath.Ext(event.Name) == ".lua" && (event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0) {
					log.Println("Reloading scripts due to change:", event.Name)
					loadLuaScripts(dir)
				}
			case err := <-watcher.Errors:
				if err != nil {
					log.Println("Watcher error:", err)
				}
			case <-ctx.Done():
				log.Println("Stopping file watcher...")
				return
			}
		}
	}()
	watcher.Add(dir)
}
