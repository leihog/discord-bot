package lua

import (
	"os"
	"testing"

	"github.com/leihog/discord-bot/internal/database"
	lua "github.com/yuin/gopher-lua"
)

func setupTestDB(t *testing.T) *database.DB {
	// Use a temporary database for testing
	dbPath := "test_kv_store.db"
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	if err := db.Initialize(); err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	// Clean up after test
	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})

	return db
}

func TestStoreSetAndGetString(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test storing and retrieving a simple string
	err := engine.StoreSet("test", "key1", lua.LString("hello world"))
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	value, err := engine.StoreGet("test", "key1")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}

	if value.String() != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", value.String())
	}
}

func TestStoreSetAndGetTable(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create a Lua table
	L := lua.NewState()
	defer L.Close()

	table := L.NewTable()
	table.RawSetString("name", lua.LString("test"))
	table.RawSetString("value", lua.LNumber(42))
	table.RawSetString("active", lua.LBool(true))

	// Test storing and retrieving a table
	err := engine.StoreSet("test", "table1", table)
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	value, err := engine.StoreGet("test", "table1")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}

	// Check if the returned value is a table
	if tbl, ok := value.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T: %s", value, value.String())
	} else {
		// Check table contents
		if name := tbl.RawGetString("name"); name.String() != "test" {
			t.Errorf("Expected name 'test', got '%s'", name.String())
		}
		if val := tbl.RawGetString("value"); val.String() != "42" {
			t.Errorf("Expected value '42', got '%s'", val.String())
		}
		if active := tbl.RawGetString("active"); active.String() != "true" {
			t.Errorf("Expected active 'true', got '%s'", active.String())
		}
	}
}

func TestStoreSetAndGetNestedTable(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create a nested Lua table
	L := lua.NewState()
	defer L.Close()

	innerTable := L.NewTable()
	innerTable.RawSetString("nested", lua.LString("value"))

	outerTable := L.NewTable()
	outerTable.RawSetString("level1", lua.LString("test"))
	outerTable.RawSetString("level2", innerTable)

	// Test storing and retrieving a nested table
	err := engine.StoreSet("test", "nested_table", outerTable)
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	value, err := engine.StoreGet("test", "nested_table")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}

	// Check if the returned value is a table
	if tbl, ok := value.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T: %s", value, value.String())
	} else {
		// Check outer table contents
		if level1 := tbl.RawGetString("level1"); level1.String() != "test" {
			t.Errorf("Expected level1 'test', got '%s'", level1.String())
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

func TestStoreDelete(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Store a value
	err := engine.StoreSet("test", "delete_key", lua.LString("to_delete"))
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	// Verify it exists
	value, err := engine.StoreGet("test", "delete_key")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}
	if value.String() != "to_delete" {
		t.Errorf("Expected 'to_delete', got '%s'", value.String())
	}

	// Delete it
	err = engine.StoreDelete("test", "delete_key")
	if err != nil {
		t.Fatalf("StoreDelete failed: %v", err)
	}

	// Verify it's gone
	value, err = engine.StoreGet("test", "delete_key")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}
	if value != lua.LNil {
		t.Errorf("Expected nil after delete, got %v", value)
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Try to get a non-existent key
	value, err := engine.StoreGet("test", "non_existent")
	if err != nil {
		t.Fatalf("StoreGet failed: %v", err)
	}
	if value != lua.LNil {
		t.Errorf("Expected nil for non-existent key, got %v", value)
	}
}

func TestStoreGetAll(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Store multiple values in the same namespace
	err := engine.StoreSet("test_all", "key1", lua.LString("value1"))
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	err = engine.StoreSet("test_all", "key2", lua.LString("value2"))
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	// Store a table
	L := lua.NewState()
	defer L.Close()
	table := L.NewTable()
	table.RawSetString("name", lua.LString("test_table"))
	table.RawSetString("value", lua.LNumber(42))

	err = engine.StoreSet("test_all", "key3", table)
	if err != nil {
		t.Fatalf("StoreSet failed: %v", err)
	}

	// Get all values from the namespace
	result, err := engine.StoreGetAll("test_all")
	if err != nil {
		t.Fatalf("StoreGetAll failed: %v", err)
	}

	// Check if the returned value is a table
	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T: %s", result, result.String())
	} else {
		// Check that all keys are present
		if key1 := tbl.RawGetString("key1"); key1.String() != "value1" {
			t.Errorf("Expected key1 'value1', got '%s'", key1.String())
		}
		if key2 := tbl.RawGetString("key2"); key2.String() != "value2" {
			t.Errorf("Expected key2 'value2', got '%s'", key2.String())
		}
		if key3 := tbl.RawGetString("key3"); key3 == lua.LNil {
			t.Error("Expected key3 to exist")
		} else if key3Tbl, ok := key3.(*lua.LTable); ok {
			if name := key3Tbl.RawGetString("name"); name.String() != "test_table" {
				t.Errorf("Expected name 'test_table', got '%s'", name.String())
			}
			if value := key3Tbl.RawGetString("value"); value.String() != "42" {
				t.Errorf("Expected value '42', got '%s'", value.String())
			}
		} else {
			t.Errorf("Expected key3 to be a table, got %T", key3)
		}
	}
}

func TestStoreGetAllEmpty(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Get all values from an empty namespace
	result, err := engine.StoreGetAll("empty_namespace")
	if err != nil {
		t.Fatalf("StoreGetAll failed: %v", err)
	}

	// Should return an empty table, not nil
	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T: %s", result, result.String())
	} else {
		// Check that the table is empty
		count := 0
		tbl.ForEach(func(key lua.LValue, value lua.LValue) {
			count++
		})
		if count != 0 {
			t.Errorf("Expected empty table, got %d items", count)
		}
	}
}
