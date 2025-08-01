// main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/fsnotify/fsnotify"
	lua "github.com/yuin/gopher-lua"
)

var (
	session               *discordgo.Session
	luaState              *lua.LState
	hookMutex             sync.Mutex
	onChannelMessageHooks []lua.LValue
	onDirectMessageHooks  []lua.LValue
	shutdownChan          chan os.Signal
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
