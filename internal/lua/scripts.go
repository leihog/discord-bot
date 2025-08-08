package lua

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
)

var hookNames = []string{
	"on_channel_message",
	"on_direct_message",
	"on_shutdown",
	"on_unload",
}

type LuaScript struct {
	Name     string
	Path     string
	Env      *lua.LTable
	OnUnload *lua.LFunction
	Commands []string
}

func (e *Engine) loadScript(path string) error {
	name := filepath.Base(path)

	code, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	L := e.state
	env := L.NewTable()

	mt := L.NewTable()
	mt.RawSetString("__index", L.Get(lua.GlobalsIndex))
	L.SetMetatable(env, mt)

	fn, err := L.LoadString(string(code))
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	script := &LuaScript{
		Name: name,
		Path: path,
		Env:  env,
	}

	e.currentScript = script
	L.Push(fn)
	L.Push(env)
	if err := L.PCall(1, lua.MultRet, nil); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	e.currentScript = nil

	// might switch to this model for hooks later. Haven't decided yet.
	// for _, hookName := range hookNames {
	// 	rawFunc := env.RawGetString(hookName)
	// 	if hookFunc, ok := rawFunc.(*lua.LFunction); ok {
	// 		e.registerScriptHook(hookName, script, hookFunc)
	// 	}
	// }

	e.scripts[name] = script

	log.Printf("Script '%s' loaded", name)
	// todo: print out how many commands and hooks the script registered
	return nil
}

// LoadScripts loads all Lua scripts from the given directory
func (e *Engine) LoadScripts(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Println("Failed to read script directory:", err)
		return
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".lua" {
			continue
		}

		scriptPath := filepath.Join(dir, f.Name())
		if err := e.loadScript(scriptPath); err != nil {
			log.Println("Failed to load script", f.Name(), ":", err)
			continue
		}
	}
}

func (e *Engine) unloadScript(name string) {
	script, ok := e.scripts[name]
	if !ok {
		log.Printf("Script '%s' not found during unload", name)
		return
	}

	if script.OnUnload != nil {
		log.Printf("Dispatching on_unload for script '%s'", name)
		e.callLuaFunction(HookInfo{
			Function: script.OnUnload,
			Script:   script,
		}, lua.LNil)
	}

	e.removeHooks(script)
	e.timer.UnregisterScriptTimers(name)
	for _, cmd := range script.Commands {
		delete(e.commands, cmd)
	}

	delete(e.scripts, script.Name)
	log.Printf("Script '%s' fully unloaded", name)
}

func (e *Engine) reloadScript(path string) error {
	name := filepath.Base(path)
	e.unloadScript(name)
	return e.loadScript(path)
}

func (e *Engine) removeHooks(script *LuaScript) {
	for name, hooks := range e.hooks {
		newHooks := hooks[:0] // reuse existing slice storage
		for _, h := range hooks {
			if h.Script != script {
				newHooks = append(newHooks, h)
			}
		}
		e.hooks[name] = newHooks
	}
}
