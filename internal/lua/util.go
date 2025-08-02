package lua

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

// jsonEncode converts a Lua table to a JSON string
func (e *Engine) jsonEncode(table *lua.LTable) (lua.LValue, error) {
	// Convert Lua table to Go map
	goMap := luaTableToMap(table)

	// Encode to JSON
	jsonBytes, err := json.Marshal(goMap)
	if err != nil {
		return lua.LNil, err
	}

	return lua.LString(string(jsonBytes)), nil
}

// jsonDecode converts a JSON string to a Lua table
func (e *Engine) jsonDecode(jsonStr string) (lua.LValue, error) {
	// Decode JSON to Go map
	var goMap map[string]any
	err := json.Unmarshal([]byte(jsonStr), &goMap)
	if err != nil {
		return lua.LNil, err
	}

	// Convert Go map to Lua table
	return goValueToLua(e.state, goMap), nil
}
