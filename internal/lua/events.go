package lua

import (
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// HTTPResult holds the result of an async HTTP request using plain Go types.
// Lua tables cannot be constructed off the dispatcher goroutine, so we carry
// raw data here and build the table inside Dispatch.
type HTTPResult struct {
	StatusCode int
	Body       string
	Headers    map[string][]string
	Err        error
}

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

// AsyncHTTPEvent is enqueued by an HTTP goroutine when its request completes.
// The Lua table is built here on the dispatcher goroutine so LState is never
// touched from outside the dispatcher.
type AsyncHTTPEvent struct {
	Callback HookInfo
	Result   HTTPResult
}

func (ae AsyncHTTPEvent) Dispatch(e *Engine) {
	result := e.state.NewTable()
	if ae.Result.Err != nil {
		result.RawSetString("error", lua.LString(ae.Result.Err.Error()))
	} else {
		result.RawSetString("status", lua.LNumber(ae.Result.StatusCode))
		result.RawSetString("body", lua.LString(ae.Result.Body))

		headersTable := e.state.NewTable()
		for key, values := range ae.Result.Headers {
			if len(values) > 0 {
				headersTable.RawSetString(key, lua.LString(values[0]))
			}
		}
		result.RawSetString("headers", headersTable)
	}
	e.callLuaFunction(ae.Callback, result)
}

func (ae AsyncHTTPEvent) Type() string {
	return "http_async"
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

// ExecResult holds the output and error from an ExecEvent.
type ExecResult struct {
	Output string
	Err    error
}

// ExecEvent runs arbitrary Lua code on the dispatcher goroutine.
// print() output and return values are captured into Result.
type ExecEvent struct {
	Code   string
	Result chan<- ExecResult
}

func (ee ExecEvent) Dispatch(e *Engine) {
	var buf strings.Builder

	origPrint := e.state.GetGlobal("print")
	e.state.SetGlobal("print", e.state.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		parts := make([]string, n)
		for i := 1; i <= n; i++ {
			parts[i-1] = L.ToStringMeta(L.Get(i)).String()
		}
		buf.WriteString(strings.Join(parts, "\t"))
		buf.WriteByte('\n')
		return 0
	}))
	defer e.state.SetGlobal("print", origPrint)

	// Try wrapping with "return" to capture expression values; fall back to
	// running the code as-is (handles statements, loops, etc.).
	fn, err := e.state.LoadString("return " + ee.Code)
	if err != nil {
		fn, err = e.state.LoadString(ee.Code)
		if err != nil {
			ee.Result <- ExecResult{Output: buf.String(), Err: err}
			return
		}
	}

	baseTop := e.state.GetTop()
	e.state.Push(fn)
	if callErr := e.state.PCall(0, lua.MultRet, nil); callErr != nil {
		e.state.SetTop(baseTop)
		ee.Result <- ExecResult{Output: buf.String(), Err: callErr}
		return
	}

	nret := e.state.GetTop() - baseTop
	if nret > 0 {
		parts := make([]string, nret)
		for i := 0; i < nret; i++ {
			parts[i] = e.state.ToStringMeta(e.state.Get(baseTop+1+i)).String()
		}
		e.state.SetTop(baseTop)
		retStr := strings.Join(parts, "\t")
		if retStr != "" {
			buf.WriteString(retStr)
		}
	}

	ee.Result <- ExecResult{Output: strings.TrimRight(buf.String(), "\n"), Err: nil}
}

func (ee ExecEvent) Type() string { return "exec" }

// snapshotEvent fetches read-only state from the dispatcher goroutine.
type snapshotEvent struct {
	kind   string
	result chan<- []string
}

func (se snapshotEvent) Dispatch(e *Engine) {
	var names []string
	switch se.kind {
	case "scripts":
		for name := range e.scripts {
			names = append(names, name)
		}
	}
	se.result <- names
}

func (se snapshotEvent) Type() string { return "snapshot_" + se.kind }
