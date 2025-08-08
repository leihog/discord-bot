package lua

import (
	"log"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// registerFunctions registers all available functions with the Lua state
func (e *Engine) registerFunctions() {
	// send_message function
	e.state.SetGlobal("send_message", e.state.NewFunction(func(L *lua.LState) int {
		channelID := L.CheckString(1)
		message := L.CheckString(2)
		_, err := e.session.ChannelMessageSend(channelID, message)
		if err != nil {
			log.Println("send_message error:", err)
		}
		return 0
	}))

	// register_command function
	e.state.SetGlobal("register_command", e.state.NewFunction(func(L *lua.LState) int {
		commandName := L.CheckString(1)
		commandDescription := L.CheckString(2)
		commandCallback := L.CheckFunction(3)
		commandCooldown := time.Duration(0) // default is no cooldown
		if L.GetTop() >= 4 {
			commandCooldown = time.Duration(L.CheckNumber(4)) * time.Second
		}

		// Validate command name
		if commandName == "" {
			log.Println("Error: Command name cannot be empty")
			return 0
		}

		// Check for invalid characters in command name
		if strings.ContainsAny(commandName, " \t\n\r") {
			log.Printf("Error: Command name '%s' contains invalid characters", commandName)
			return 0
		}

		e.cmdMutex.Lock()
		defer e.cmdMutex.Unlock()

		if existingCommand, exists := e.commands[commandName]; exists {
			log.Printf("Command '%s' already registered by script '%s'", commandName, existingCommand.Callback.Script)
			return 0
		}

		e.commands[commandName] = &Command{
			Name:        commandName,
			Description: commandDescription,
			Callback: HookInfo{
				Function: commandCallback,
				Script:   e.currentScript,
			},
			Cooldown: commandCooldown,
			LastUsed: time.Time{}, // Zero time for initial state
		}

		e.currentScript.Commands = append(e.currentScript.Commands, commandName)

		log.Printf("Command '%s' registered by script '%s'", commandName, e.currentScript.Name)
		return 0
	}))

	// get_commands function
	e.state.SetGlobal("get_commands", e.state.NewFunction(func(L *lua.LState) int {
		e.cmdMutex.Lock()
		defer e.cmdMutex.Unlock()

		commandsTable := L.NewTable()
		for name, cmd := range e.commands {
			cmdTable := L.NewTable()
			cmdTable.RawSetString("name", lua.LString(cmd.Name))
			cmdTable.RawSetString("description", lua.LString(cmd.Description))
			cmdTable.RawSetString("script", lua.LString(cmd.Callback.Script.Name))
			cmdTable.RawSetString("cooldown", lua.LNumber(cmd.Cooldown.Seconds()))
			commandsTable.RawSetString(name, cmdTable)
		}

		L.Push(commandsTable)
		return 1
	}))

	// register_hook function
	e.state.SetGlobal("register_hook", e.state.NewFunction(func(L *lua.LState) int {
		hookName := L.CheckString(1)
		hookFunc := L.CheckFunction(2)

		e.hookMutex.Lock()
		defer e.hookMutex.Unlock()

		switch hookName {
		case "on_channel_message", "on_direct_message", "on_shutdown":
			e.hooks[hookName] = append(e.hooks[hookName], HookInfo{
				Function: hookFunc,
				Script:   e.currentScript,
			})
		case "on_unload":
			e.currentScript.OnUnload = hookFunc
		default:
			log.Println("Unknown hook name:", hookName)
		}
		return 0
	}))

	// store_set function
	e.state.SetGlobal("store_set", e.state.NewFunction(func(L *lua.LState) int {
		namespace := L.CheckString(1)
		key := L.CheckString(2)
		value := L.CheckAny(3)

		if err := e.StoreSet(namespace, key, value); err != nil {
			log.Println("store_set error:", err)
		}
		return 0
	}))

	// store_get function
	e.state.SetGlobal("store_get", e.state.NewFunction(func(L *lua.LState) int {
		namespace := L.CheckString(1)
		key := L.CheckString(2)

		value, err := e.StoreGet(namespace, key)
		if err != nil {
			log.Println("store_get error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(value)
		}
		return 1
	}))

	// store_delete function
	e.state.SetGlobal("store_delete", e.state.NewFunction(func(L *lua.LState) int {
		namespace := L.CheckString(1)
		key := L.CheckString(2)

		if err := e.StoreDelete(namespace, key); err != nil {
			log.Println("store_delete error:", err)
		}
		return 0
	}))

	// store_get_all function
	e.state.SetGlobal("store_get_all", e.state.NewFunction(func(L *lua.LState) int {
		namespace := L.CheckString(1)

		value, err := e.StoreGetAll(namespace)
		if err != nil {
			log.Println("store_get_all error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(value)
		}
		return 1
	}))

	// http_get function
	e.state.SetGlobal("http_get", e.state.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		var options *lua.LTable
		if L.GetTop() > 1 {
			options = L.CheckTable(2)
		}

		result, err := e.httpGet(url, options)
		if err != nil {
			log.Println("http_get error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(result)
		}
		return 1
	}))

	// http_post function
	e.state.SetGlobal("http_post", e.state.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		body := L.CheckString(2)
		var options *lua.LTable
		if L.GetTop() > 2 {
			options = L.CheckTable(3)
		}

		result, err := e.httpPost(url, body, options)
		if err != nil {
			log.Println("http_post error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(result)
		}
		return 1
	}))

	// json_encode function
	e.state.SetGlobal("json_encode", e.state.NewFunction(func(L *lua.LState) int {
		table := L.CheckTable(1)

		result, err := e.jsonEncode(table)
		if err != nil {
			log.Println("json_encode error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(result)
		}
		return 1
	}))

	// json_decode function
	e.state.SetGlobal("json_decode", e.state.NewFunction(func(L *lua.LState) int {
		jsonStr := L.CheckString(1)

		result, err := e.jsonDecode(jsonStr)
		if err != nil {
			log.Println("json_decode error:", err)
			L.Push(lua.LNil)
		} else {
			L.Push(result)
		}
		return 1
	}))

	// log function
	e.state.SetGlobal("log", e.state.NewFunction(func(L *lua.LState) int {
		message := L.CheckString(1)
		log.Printf("[Lua Script] %s", message)
		return 0
	}))

	// register_timer function (one-shot timer)
	e.state.SetGlobal("call_later", e.state.NewFunction(func(L *lua.LState) int {
		seconds := L.CheckNumber(1)
		callback := L.CheckFunction(2)
		var data lua.LValue = lua.LNil
		if L.GetTop() > 2 {
			data = L.CheckAny(3)
		}

		// Get the current script name
		scriptName := e.currentScript

		timerID := e.timer.RegisterTimer(float64(seconds), callback, data, scriptName)
		L.Push(lua.LString(timerID))
		return 1
	}))

	// register_repeating_timer function
	e.state.SetGlobal("register_timer", e.state.NewFunction(func(L *lua.LState) int {
		seconds := L.CheckNumber(1)
		callback := L.CheckFunction(2)
		var data lua.LValue = lua.LNil
		if L.GetTop() > 2 {
			data = L.CheckAny(3)
		}

		timerID := e.timer.RegisterRepeatingTimer(float64(seconds), callback, data, e.currentScript)
		L.Push(lua.LString(timerID))
		return 1
	}))

	// unregister_timer function
	e.state.SetGlobal("unregister_timer", e.state.NewFunction(func(L *lua.LState) int {
		timerID := L.CheckString(1)

		success := e.timer.UnregisterTimer(timerID)
		L.Push(lua.LBool(success))
		return 1
	}))
}
