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
		e.hookMutex.Lock()
		defer e.hookMutex.Unlock()
		switch hookName {
		case "on_channel_message":
			e.onChannelMessageHooks = append(e.onChannelMessageHooks, hookFunc)
		case "on_direct_message":
			e.onDirectMessageHooks = append(e.onDirectMessageHooks, hookFunc)
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
}
