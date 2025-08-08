package lua

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	lua "github.com/yuin/gopher-lua"

	"github.com/leihog/discord-bot/internal/database"
)

// todo optimize the way we handle hooks. I'm not entirely happy with the current implementation.

// HookInfo contains information about a registered hook
type HookInfo struct {
	Function lua.LValue
	Script   *LuaScript
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
	state     *lua.LState
	db        *database.DB
	session   *discordgo.Session
	hookMutex sync.Mutex
	hooks     map[string][]HookInfo

	scripts       map[string]*LuaScript
	currentScript *LuaScript

	// Event queue system
	eventQueue   chan Event
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
		eventQueue: make(chan Event, 200), // Buffer for 200 events
		hooks:      make(map[string][]HookInfo),
		commands:   make(map[string]*Command),
		scripts:    make(map[string]*LuaScript),
	}
	//engine.scriptManager = NewScriptManager(engine)
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

// callLuaFunction calls a Lua function with the given data
func (e *Engine) callLuaFunction(fn HookInfo, data lua.LValue) {
	e.currentScript = fn.Script
	defer func() { e.currentScript = nil }()

	if err := e.state.CallByParam(lua.P{
		Fn:      fn.Function,
		NRet:    0,
		Protect: true,
	}, data); err != nil {
		log.Printf("Lua error in script '%s': %v", fn.Script.Name, err)
	}
}

// dispatcher runs the main Lua event processing loop
func (e *Engine) dispatcher() {
	defer e.dispatcherWg.Done()

	for event := range e.eventQueue {
		event.Dispatch(e)
	}

	log.Println("Event queue closed and drained")
}

func (e *Engine) enqueueEvent(event Event, source string) {
	select {
	case e.eventQueue <- event:
		// Event queued successfully
	// todo test using timeout
	// case <-time.After(100 * time.Millisecond): // we could use this to drop events if the queue is still full after 100ms
	default:
		log.Printf("Warning: Lua event queue full, dropping %s event from '%s'", event.Type(), source)
	}
}

func (e *Engine) enqueueMessageHooks(m *discordgo.MessageCreate) {
	data := e.state.NewTable()
	data.RawSetString("content", lua.LString(m.Content))
	data.RawSetString("channel_id", lua.LString(m.ChannelID))
	data.RawSetString("author", lua.LString(m.Author.Username))

	var eventType string
	if m.GuildID == "" {
		eventType = "on_direct_message"
	} else {
		eventType = "on_channel_message"
	}

	event := BotEvent{
		Data:      data,
		EventType: eventType,
	}

	e.enqueueEvent(event, m.Author.Username)
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

	event := CommandEvent{
		CommandName: commandName,
		CommandData: data,
		Callback:    cmd.Callback,
	}

	e.enqueueEvent(event, m.Author.Username)
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

	// Enqueue shutdown event
	event := BotEvent{
		Data:      data,
		EventType: "on_shutdown",
	}
	e.enqueueEvent(event, "shutdown")

	log.Println("Waiting for event queue to drain...")

	close(e.eventQueue) // stop accepting new events and drain the queue
	e.dispatcherWg.Wait()

	// unload all scripts
	for name := range e.scripts {
		e.unloadScript(name)
	}

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
