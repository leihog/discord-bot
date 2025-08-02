package lua

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestJsonEncodeBasic(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create a simple Lua table
	L := lua.NewState()
	defer L.Close()
	table := L.NewTable()
	table.RawSetString("name", lua.LString("test"))
	table.RawSetString("value", lua.LNumber(42))
	table.RawSetString("active", lua.LBool(true))

	// Test JSON encoding
	result, err := engine.jsonEncode(table)
	if err != nil {
		t.Fatalf("jsonEncode failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if jsonStr, ok := result.(lua.LString); !ok {
		t.Errorf("Expected string, got %T", result)
	} else {
		expected := `{"active":"true","name":"test","value":"42"}`
		if jsonStr.String() != expected {
			t.Errorf("Expected %s, got %s", expected, jsonStr.String())
		}
	}
}

func TestJsonEncodeComplex(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create a complex Lua table with nested structure
	L := lua.NewState()
	defer L.Close()

	// Inner table
	innerTable := L.NewTable()
	innerTable.RawSetString("nested", lua.LString("value"))

	// Outer table
	outerTable := L.NewTable()
	outerTable.RawSetString("level1", lua.LString("test"))
	outerTable.RawSetString("level2", innerTable)
	outerTable.RawSetString("number", lua.LNumber(123))

	// Test JSON encoding
	result, err := engine.jsonEncode(outerTable)
	if err != nil {
		t.Fatalf("jsonEncode failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if jsonStr, ok := result.(lua.LString); !ok {
		t.Errorf("Expected string, got %T", result)
	} else {
		// Check that it contains expected fields
		jsonString := jsonStr.String()
		if !contains(jsonString, "level1") {
			t.Errorf("Expected to contain 'level1', got %s", jsonString)
		}
		if !contains(jsonString, "level2") {
			t.Errorf("Expected to contain 'level2', got %s", jsonString)
		}
		if !contains(jsonString, "nested") {
			t.Errorf("Expected to contain 'nested', got %s", jsonString)
		}
	}
}

func TestJsonDecodeBasic(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test JSON decoding
	jsonString := `{"name":"test","value":42,"active":true}`
	result, err := engine.jsonDecode(jsonString)
	if err != nil {
		t.Fatalf("jsonDecode failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check table contents
		if name := tbl.RawGetString("name"); name.String() != "test" {
			t.Errorf("Expected name 'test', got '%s'", name.String())
		}
		if value := tbl.RawGetString("value"); value.String() != "42" {
			t.Errorf("Expected value '42', got '%s'", value.String())
		}
		if active := tbl.RawGetString("active"); active.String() != "true" {
			t.Errorf("Expected active 'true', got '%s'", active.String())
		}
	}
}

func TestJsonDecodeComplex(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test JSON decoding with nested structure
	jsonString := `{"level1":"test","level2":{"nested":"value"},"number":123}`
	result, err := engine.jsonDecode(jsonString)
	if err != nil {
		t.Fatalf("jsonDecode failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check outer table contents
		if level1 := tbl.RawGetString("level1"); level1.String() != "test" {
			t.Errorf("Expected level1 'test', got '%s'", level1.String())
		}
		if number := tbl.RawGetString("number"); number.String() != "123" {
			t.Errorf("Expected number '123', got '%s'", number.String())
		}

		// Check nested table
		if level2 := tbl.RawGetString("level2"); level2 != lua.LNil {
			if innerTbl, ok := level2.(*lua.LTable); ok {
				if nested := innerTbl.RawGetString("nested"); nested.String() != "value" {
					t.Errorf("Expected nested 'value', got '%s'", nested.String())
				}
			} else {
				t.Errorf("Expected level2 to be a table, got %T", level2)
			}
		} else {
			t.Error("Expected level2 to exist")
		}
	}
}

func TestJsonRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create a complex Lua table
	L := lua.NewState()
	defer L.Close()

	innerTable := L.NewTable()
	innerTable.RawSetString("nested", lua.LString("value"))

	originalTable := L.NewTable()
	originalTable.RawSetString("name", lua.LString("test"))
	originalTable.RawSetString("number", lua.LNumber(42))
	originalTable.RawSetString("nested", innerTable)

	// Encode
	jsonString, err := engine.jsonEncode(originalTable)
	if err != nil {
		t.Fatalf("jsonEncode failed: %v", err)
	}

	// Decode
	decodedTable, err := engine.jsonDecode(jsonString.String())
	if err != nil {
		t.Fatalf("jsonDecode failed: %v", err)
	}

	// Encode again
	finalJsonString, err := engine.jsonEncode(decodedTable.(*lua.LTable))
	if err != nil {
		t.Fatalf("second jsonEncode failed: %v", err)
	}

	// Check that the roundtrip preserved the data
	if jsonString.String() != finalJsonString.String() {
		t.Errorf("Roundtrip failed: original=%s, final=%s",
			jsonString.String(), finalJsonString.String())
	}
}

func TestJsonDecodeInvalid(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test invalid JSON
	invalidJson := `{"name":"test",invalid}`
	result, err := engine.jsonDecode(invalidJson)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}

	if result != lua.LNil {
		t.Error("Expected nil result for invalid JSON")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
