package lua

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bwmarrin/discordgo"
	lua "github.com/yuin/gopher-lua"

	"github.com/leihog/discord-bot/internal/database"
)

// Engine manages the Lua scripting environment
type Engine struct {
	state                 *lua.LState
	db                    *database.DB
	session               *discordgo.Session
	hookMutex             sync.Mutex
	onChannelMessageHooks []lua.LValue
	onDirectMessageHooks  []lua.LValue
}

// New creates a new Lua engine
func New(db *database.DB, session *discordgo.Session) *Engine {
	return &Engine{
		state:   lua.NewState(),
		db:      db,
		session: session,
	}
}

// Initialize sets up the Lua engine with all functions
func (e *Engine) Initialize() {
	e.registerFunctions()
}

// LoadScripts loads all Lua scripts from the given directory
func (e *Engine) LoadScripts(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Println("Failed to read script directory:", err)
		return
	}

	// Clear existing hooks
	e.onChannelMessageHooks = nil
	e.onDirectMessageHooks = nil

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".lua" {
			scriptPath := filepath.Join(dir, f.Name())
			if err := e.state.DoFile(scriptPath); err != nil {
				log.Println("Failed to load script", f.Name(), ":", err)
			}
		}
	}
}

// ProcessMessage processes a Discord message through all registered hooks
func (e *Engine) ProcessMessage(m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	data := e.state.NewTable()
	data.RawSetString("content", lua.LString(m.Content))
	data.RawSetString("channel_id", lua.LString(m.ChannelID))
	data.RawSetString("author", lua.LString(m.Author.Username))

	e.hookMutex.Lock()
	defer e.hookMutex.Unlock()

	var hooks []lua.LValue
	if m.GuildID == "" {
		hooks = e.onDirectMessageHooks
	} else {
		hooks = e.onChannelMessageHooks
	}

	for _, fn := range hooks {
		if err := e.state.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, data); err != nil {
			log.Println("Lua hook error:", err)
		}
	}
}

// Close closes the Lua engine
func (e *Engine) Close() {
	if e.state != nil {
		e.state.Close()
	}
}
