package lua

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

// Event is the common interface for all events handled by the engine
type Event interface {
	Dispatch(e *Engine)
	Type() string
}

// BotEvent is an event triggered by discord that LuaScripts can subscribe to
type BotEvent struct {
	Data      lua.LValue
	EventType string // "on_channel_message", "on_direct_message", etc.
}

func (be BotEvent) Dispatch(e *Engine) {
	for _, hook := range e.hooks[be.EventType] {
		// make this a debug log later so it's not spammy
		log.Printf("Dispatching %s for script %s", be.EventType, hook.Script.Name)
		e.callLuaFunction(hook, be.Data)
	}
}

func (be BotEvent) Type() string {
	return be.EventType
}

type TimerEvent struct {
	TimerID   string
	TimerData lua.LValue
	Callback  HookInfo
}

func (te TimerEvent) Dispatch(e *Engine) {
	log.Printf("Dispatching timer %s for script %s", te.TimerID, te.Callback.Script.Name)
	e.callLuaFunction(te.Callback, te.TimerData)
}

func (te TimerEvent) Type() string {
	return "timer(" + te.TimerID + ")"
}

type CommandEvent struct {
	CommandName string
	CommandData lua.LValue
	Callback    HookInfo
}

func (ce CommandEvent) Dispatch(e *Engine) {
	e.callLuaFunction(ce.Callback, ce.CommandData)
}

func (ce CommandEvent) Type() string {
	return "command(" + ce.CommandName + ")"
}

// ScriptEvent represents an internal system event to manage Lua scripts
// todo: Do I want to add an onLoad event?
type ScriptEvent struct {
	Action     string // "reload", "unload", etc.
	ScriptName string
}

func (se ScriptEvent) Dispatch(e *Engine) {
	switch se.Action {
	case "unload":
		e.unloadScript(se.ScriptName)

	case "reload":
		e.reloadScript(se.ScriptName)

	default:
		log.Printf("Unknown ScriptEvent action: %s", se.Action)
	}
}

func (se ScriptEvent) Type() string {
	return "script_" + se.Action
}
