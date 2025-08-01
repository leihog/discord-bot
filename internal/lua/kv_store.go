package lua

import (
	"database/sql"
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

// StoreSet stores a value in the key-value store
func (e *Engine) StoreSet(namespace, key string, value lua.LValue) error {
	var valStr string

	if tbl, ok := value.(*lua.LTable); ok {
		// Convert Lua table to Go map
		goVal := luaTableToMap(tbl)
		jsonBytes, err := json.Marshal(goVal)
		if err != nil {
			return err
		}
		valStr = string(jsonBytes)
	} else {
		valStr = value.String()
	}

	_, err := e.db.Exec(`INSERT INTO kv_store(namespace, key, value) VALUES (?, ?, ?) 
		ON CONFLICT(namespace, key) DO UPDATE SET value=excluded.value`, namespace, key, valStr)
	return err
}

// StoreGet retrieves a value from the key-value store
func (e *Engine) StoreGet(namespace, key string) (lua.LValue, error) {
	row := e.db.QueryRow(`SELECT value FROM kv_store WHERE namespace = ? AND key = ?`, namespace, key)
	var valStr string
	err := row.Scan(&valStr)
	if err == sql.ErrNoRows {
		return lua.LNil, nil
	} else if err != nil {
		return lua.LNil, err
	}

	// Try to decode as JSON object
	var decoded any
	if json.Unmarshal([]byte(valStr), &decoded) == nil {
		return goValueToLua(e.state, decoded), nil
	} else {
		return lua.LString(valStr), nil
	}
}

// StoreDelete removes a value from the key-value store
func (e *Engine) StoreDelete(namespace, key string) error {
	_, err := e.db.Exec(`DELETE FROM kv_store WHERE namespace = ? AND key = ?`, namespace, key)
	return err
}

// luaTableToMap converts a Lua table to a Go map
func luaTableToMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		k := key.String()
		switch v := value.(type) {
		case *lua.LTable:
			result[k] = luaTableToMap(v)
		default:
			result[k] = v.String()
		}
	})
	return result
}

// goValueToLua converts a Go value to a Lua value with proper table reconstruction
func goValueToLua(L *lua.LState, v any) lua.LValue {
	switch val := v.(type) {
	case map[string]any:
		tbl := L.NewTable()
		for k, v2 := range val {
			tbl.RawSetString(k, goValueToLua(L, v2))
		}
		return tbl
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case nil:
		return lua.LNil
	default:
		return lua.LString("unsupported")
	}
}
