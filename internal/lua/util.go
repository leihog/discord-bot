package lua

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

// jsonEncode converts a Lua table to a JSON string
func (e *Engine) jsonEncode(table *lua.LTable) (lua.LValue, error) {
	jsonBytes, err := json.Marshal(luaTableToGo(table))
	if err != nil {
		return lua.LNil, err
	}
	return lua.LString(string(jsonBytes)), nil
}

// jsonDecode converts a JSON string to a Lua value (table, array, string, number, or bool)
func (e *Engine) jsonDecode(jsonStr string) (lua.LValue, error) {
	var v any
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return lua.LNil, err
	}
	return goValueToLua(e.state, v), nil
}
