package lua

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Command represents a scripted Bot command
type Command struct {
	Name          string
	Description   string
	Callback      HookInfo
	Cooldown      time.Duration
	LastUsed      time.Time // Global cooldown for the command
	lastUsedMutex sync.RWMutex
}

// Engine manages the Lua scripting environment
type Engine struct {
	state                 *lua.LState
	db                    *database.DB
	session               *discordgo.Session
	hookMutex             sync.Mutex
	onChannelMessageHooks []HookInfo
	onDirectMessageHooks  []HookInfo
	onShutdownHooks       []HookInfo
	currentScript         string // Track the currently executing script

	// Event queue system
	eventQueue   chan LuaEvent
	ctx          context.Context
	cancel       context.CancelFunc
	dispatcherWg sync.WaitGroup

	// Timer system
	timer *Timer

	// Command system
	commands map[string]*Command
	cmdMutex sync.Mutex

	// Shutdown state
	shutdownMutex  sync.RWMutex
	isShuttingDown bool
}

// New creates a new Lua engine
func New(db *database.DB, session *discordgo.Session) *Engine {
	engine := &Engine{
		state:      lua.NewState(),
		db:         db,
		session:    session,
		eventQueue: make(chan LuaEvent, 200), // Buffer for 200 events
		commands:   make(map[string]*Command),
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
	e.dispatcherWg.Add(1)
	go e.dispatcher()
}

// dispatcher runs the main Lua event processing loop
func (e *Engine) dispatcher() {
	defer e.dispatcherWg.Done()

	for event := range e.eventQueue {
		e.processEvent(event)
	}

	log.Println("Lua event queue closed and drained")
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
	e.hookMutex.Lock()
	// reusing the slices by using e.onChannelMessageHooks[:0] might be slightly more performant when the scripts are reloaded often
	// but now while the hooks are changing so much this is more performant since the memory is freed.
	e.onChannelMessageHooks = nil
	e.onDirectMessageHooks = nil
	e.onShutdownHooks = nil
	e.hookMutex.Unlock()

	e.cmdMutex.Lock()
	e.commands = make(map[string]*Command)
	e.cmdMutex.Unlock()

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

func (e *Engine) enqueueLuaEvent(event LuaEvent, source string) {
	// Check if we're shutting down and this isn't a shutdown event
	e.shutdownMutex.RLock()
	if e.isShuttingDown && event.EventType != "shutdown" {
		e.shutdownMutex.RUnlock()
		log.Printf("Dropping %s event from '%s' - engine is shutting down", event.EventType, source)
		return
	}
	e.shutdownMutex.RUnlock()

	select {
	case e.eventQueue <- event:
		// Event queued successfully
	// todo test using timeout
	// case <-time.After(100 * time.Millisecond): // we could use this to drop events if the queue is still full after 100ms
	default:
		log.Printf("Warning: Lua event queue full, dropping %s event from '%s'", event.EventType, source)
	}
}

func (e *Engine) enqueueMessageHooks(m *discordgo.MessageCreate) {
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

		e.enqueueLuaEvent(event, hook.Script)
	}
}

func (e *Engine) tryHandleCommand(content string, m *discordgo.MessageCreate) bool {
	parts := strings.Fields(content)
	commandName := strings.TrimPrefix(parts[0], "!")

	e.cmdMutex.Lock()
	cmd, exists := e.commands[commandName]
	e.cmdMutex.Unlock()
	if !exists {
		return false
	}

	cmd.lastUsedMutex.RLock()
	lastUsed := cmd.LastUsed
	cmd.lastUsedMutex.RUnlock()

	if time.Since(lastUsed) < cmd.Cooldown {
		log.Printf("Command '%s' on cooldown", commandName)
		return true
	}

	cmd.lastUsedMutex.Lock()
	cmd.LastUsed = time.Now()
	cmd.lastUsedMutex.Unlock()

	args := e.state.NewTable()
	for i, arg := range parts {
		args.RawSetInt(i+1, lua.LString(arg))
	}

	data := e.state.NewTable()
	data.RawSetString("args", args)
	data.RawSetString("channel_id", lua.LString(m.ChannelID))
	data.RawSetString("author", lua.LString(m.Author.Username))

	event := LuaEvent{
		Hook:      cmd.Callback,
		Data:      data,
		EventType: "command",
	}

	e.enqueueLuaEvent(event, m.Author.Username)
	return true
}

// ProcessMessage processes a Discord message through all registered hooks
func (e *Engine) ProcessMessage(m *discordgo.MessageCreate) {
	// Check if we're shutting down
	if e.IsShuttingDown() {
		return
	}

	if m.Author.Bot {
		return
	}

	// Check for commands
	content := strings.TrimSpace(m.Content)
	if strings.HasPrefix(content, "!") {
		if e.tryHandleCommand(content, m) {
			return
		}
	}

	e.enqueueMessageHooks(m)
}

// Close closes the Lua engine
func (e *Engine) Close() {
	e.shutdownMutex.Lock()
	e.isShuttingDown = true
	e.shutdownMutex.Unlock()

	// Timers create events, so we need to stop them first
	if e.timer != nil {
		e.timer.StopAll()
	}

	log.Println("Triggering shutdown events in Lua scripts...")

	// Create shutdown event data
	data := e.state.NewTable()
	data.RawSetString("reason", lua.LString("graceful_shutdown"))

	// Trigger shutdown hooks
	e.hookMutex.Lock()
	for _, hook := range e.onShutdownHooks {
		event := LuaEvent{
			Hook:      hook,
			Data:      data,
			EventType: "shutdown",
		}
		e.enqueueLuaEvent(event, hook.Script)
	}
	e.hookMutex.Unlock()

	log.Println("Shutdown events queued, waiting for event queue to drain...")

	close(e.eventQueue) // stop accepting new events and drain the queue
	e.dispatcherWg.Wait()

	if e.state != nil {
		e.state.Close()
	}
}

// IsShuttingDown returns true if the engine is in shutdown mode
func (e *Engine) IsShuttingDown() bool {
	e.shutdownMutex.RLock()
	defer e.shutdownMutex.RUnlock()
	return e.isShuttingDown
}
