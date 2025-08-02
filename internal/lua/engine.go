package lua

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bwmarrin/discordgo"
	lua "github.com/yuin/gopher-lua"

	"github.com/leihog/discord-bot/internal/database"
)

// HookInfo contains information about a registered hook
type HookInfo struct {
	Function lua.LValue
	Script   string
}

// LuaEvent represents an event to be processed by the Lua engine
type LuaEvent struct {
	Hook      HookInfo
	Data      lua.LValue
	EventType string // "message", "timer", etc.
}

// Engine manages the Lua scripting environment
type Engine struct {
	state                 *lua.LState
	db                    *database.DB
	session               *discordgo.Session
	hookMutex             sync.Mutex
	onChannelMessageHooks []HookInfo
	onDirectMessageHooks  []HookInfo
	currentScript         string // Track the currently executing script

	// Event queue system
	eventQueue chan LuaEvent
	ctx        context.Context
	cancel     context.CancelFunc

	// Timer system
	timer *Timer
}

// New creates a new Lua engine
func New(db *database.DB, session *discordgo.Session) *Engine {
	engine := &Engine{
		state:      lua.NewState(),
		db:         db,
		session:    session,
		eventQueue: make(chan LuaEvent, 200), // Buffer for 200 events
	}
	engine.timer = NewTimer(engine)
	return engine
}

// Initialize sets up the Lua engine with all functions
func (e *Engine) Initialize() {
	e.registerFunctions()
}

// Start starts the Lua event dispatcher
func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)
	go e.dispatcher()
}

// dispatcher runs the main Lua event processing loop
func (e *Engine) dispatcher() {
	for {
		select {
		case event := <-e.eventQueue:
			e.processEvent(event)
		case <-e.ctx.Done():
			log.Println("Shutdown signal received, draining Lua event queue")
			for {
				select {
				case event := <-e.eventQueue:
					e.processEvent(event)
				default:
					log.Println("Lua event queue fully drained, shutting down")
					return
				}
			}
		}
	}
}

// processEvent processes a single Lua event
func (e *Engine) processEvent(event LuaEvent) {
	// Set the current script for error reporting
	e.currentScript = event.Hook.Script

	// Execute the hook function
	if err := e.state.CallByParam(lua.P{Fn: event.Hook.Function, NRet: 0, Protect: true}, event.Data); err != nil {
		log.Printf("Lua hook error in script '%s': %v", event.Hook.Script, err)
	}

	// Clear the current script
	e.currentScript = ""
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
		if filepath.Ext(f.Name()) != ".lua" {
			continue
		}

		scriptPath := filepath.Join(dir, f.Name())
		code, err := os.ReadFile(scriptPath)
		if err != nil {
			log.Println("Failed to read script", f.Name(), ":", err)
			continue
		}

		// Create a new environment for this script
		env := e.state.NewTable()

		mt := e.state.NewTable()
		mt.RawSetString("__index", e.state.Get(lua.GlobalsIndex))
		e.state.SetMetatable(env, mt)

		// Load and run the script in its environment
		fn, err := e.state.LoadString(string(code))
		if err != nil {
			log.Println("Failed to compile script", f.Name(), ":", err)
			continue
		}

		// Track the current script being executed
		e.currentScript = f.Name()

		e.state.Push(fn)
		e.state.Push(env)
		if err := e.state.PCall(1, lua.MultRet, nil); err != nil {
			log.Println("Failed to run script", f.Name(), ":", err)
		}

		// Clear the current script after execution
		e.currentScript = ""
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

	var hooks []HookInfo
	if m.GuildID == "" {
		hooks = e.onDirectMessageHooks
	} else {
		hooks = e.onChannelMessageHooks
	}

	// Enqueue events instead of executing directly
	for _, hook := range hooks {
		event := LuaEvent{
			Hook:      hook,
			Data:      data,
			EventType: "message",
		}

		select {
		case e.eventQueue <- event:
			// Event queued successfully
		// case <-time.After(100 * time.Millisecond): // we could use this to drop events if the queue is still full after 100ms
		default:
			log.Printf("Warning: Lua event queue full, dropping event from script '%s'", hook.Script)
		}
	}
}

// Close closes the Lua engine
func (e *Engine) Close() {
	if e.cancel != nil {
		e.cancel()
	}
	if e.timer != nil {
		e.timer.StopAll()
	}
	if e.state != nil {
		e.state.Close()
	}
}
