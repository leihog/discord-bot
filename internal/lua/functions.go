package lua

import (
	"log"

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

	// register_hook function
	e.state.SetGlobal("register_hook", e.state.NewFunction(func(L *lua.LState) int {
		hookName := L.CheckString(1)
		hookFunc := L.CheckFunction(2)

		// Get the current script name
		scriptName := e.currentScript

		e.hookMutex.Lock()
		defer e.hookMutex.Unlock()
		switch hookName {
		case "on_channel_message":
			e.onChannelMessageHooks = append(e.onChannelMessageHooks, HookInfo{
				Function: hookFunc,
				Script:   scriptName,
			})
		case "on_direct_message":
			e.onDirectMessageHooks = append(e.onDirectMessageHooks, HookInfo{
				Function: hookFunc,
				Script:   scriptName,
			})
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
}
