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
		goVal := luaTableToGo(tbl)
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

// StoreGetAll retrieves all values from a namespace
func (e *Engine) StoreGetAll(namespace string) (lua.LValue, error) {
	rows, err := e.db.Query(`SELECT key, value FROM kv_store WHERE namespace = ?`, namespace)
	if err != nil {
		return lua.LNil, err
	}
	defer rows.Close()

	result := e.state.NewTable()

	for rows.Next() {
		var key, valStr string
		if err := rows.Scan(&key, &valStr); err != nil {
			return lua.LNil, err
		}

		// Try to decode as JSON object
		var decoded any
		if json.Unmarshal([]byte(valStr), &decoded) == nil {
			result.RawSetString(key, goValueToLua(e.state, decoded))
		} else {
			result.RawSetString(key, lua.LString(valStr))
		}
	}

	if err := rows.Err(); err != nil {
		return lua.LNil, err
	}

	return result, nil
}

// luaTableToMap is a backward-compatible wrapper returning map[string]any.
// Prefer luaTableToGo when the table may be a sequence.
func luaTableToMap(tbl *lua.LTable) map[string]any {
	if m, ok := luaTableToGo(tbl).(map[string]any); ok {
		return m
	}
	// Array table: fall back to string-keyed map (legacy behaviour).
	m := make(map[string]any)
	tbl.ForEach(func(k, v lua.LValue) {
		m[k.String()] = luaToGo(v)
	})
	return m
}

// luaToGo converts any Lua value to its Go equivalent.
func luaToGo(value lua.LValue) any {
	switch v := value.(type) {
	case *lua.LTable:
		return luaTableToGo(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case lua.LBool:
		return bool(v)
	default:
		if value == lua.LNil {
			return nil
		}
		return v.String()
	}
}

// luaTableToGo converts a Lua table to either a []any (sequence) or map[string]any (hash).
// Tables whose only keys are consecutive integers starting at 1 are treated as arrays,
// preserving round-trip fidelity through JSON so that the Lua # operator works after retrieval.
func luaTableToGo(tbl *lua.LTable) any {
	n := tbl.MaxN()
	if n > 0 {
		isSeq := true
		tbl.ForEach(func(k, _ lua.LValue) {
			if _, ok := k.(lua.LNumber); !ok {
				isSeq = false
			}
		})
		if isSeq {
			arr := make([]any, n)
			for i := 1; i <= n; i++ {
				arr[i-1] = luaToGo(tbl.RawGetInt(i))
			}
			return arr
		}
	}
	m := make(map[string]any)
	tbl.ForEach(func(k, v lua.LValue) {
		m[k.String()] = luaToGo(v)
	})
	return m
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
	case []any:
		tbl := L.NewTable()
		for i, v2 := range val {
			tbl.RawSetInt(i+1, goValueToLua(L, v2))
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
