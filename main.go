// main.go
package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/fsnotify/fsnotify"
	"github.com/yuin/gopher-lua"
)

var (
	session   *discordgo.Session
	luaState  *lua.LState
	hookMutex sync.Mutex
	onMessageHooks []lua.LValue
)

func main() {
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}

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
	defer session.Close()

	luaState = lua.NewState()
	defer luaState.Close()
	registerLuaFunctions(luaState)
	loadLuaScripts("lua/scripts")
	watchLuaScripts("lua/scripts")

	log.Println("Bot is now running. Press CTRL+C to exit.")
	select {}
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
		if hookName == "message_create" {
			hookMutex.Lock()
			defer hookMutex.Unlock()
			onMessageHooks = append(onMessageHooks, hookFunc)
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
	data.RawSetString("is_dm", lua.LBool(m.GuildID == ""))

	hookMutex.Lock()
	defer hookMutex.Unlock()
	for _, fn := range onMessageHooks {
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

	onMessageHooks = nil

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".lua" {
			scriptPath := filepath.Join(dir, f.Name())
			if err := luaState.DoFile(scriptPath); err != nil {
				log.Println("Failed to load script", f.Name(), ":", err)
			}
		}
	}
}

func watchLuaScripts(dir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("File watcher error:", err)
		return
	}
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if filepath.Ext(event.Name) == ".lua" && (event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0) {
					log.Println("Reloading scripts due to change:", event.Name)
					loadLuaScripts(dir)
				}
			case err := <-watcher.Errors:
				log.Println("Watcher error:", err)
			}
		}
	}()
	watcher.Add(dir)
}


