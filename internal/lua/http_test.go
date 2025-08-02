package lua

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestHttpGetBasic(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test basic HTTP GET
	result, err := engine.httpGet("https://httpbin.org/get", nil)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check status
		if status := tbl.RawGetString("status"); status == lua.LNil {
			t.Error("Expected status field")
		} else if statusNum, ok := status.(lua.LNumber); !ok {
			t.Errorf("Expected status to be number, got %T", status)
		} else if statusNum != 200 {
			t.Errorf("Expected status 200, got %v", statusNum)
		}

		// Check body
		if body := tbl.RawGetString("body"); body == lua.LNil {
			t.Error("Expected body field")
		}

		// Check headers
		if headers := tbl.RawGetString("headers"); headers == lua.LNil {
			t.Error("Expected headers field")
		}
	}
}

func TestHttpGetWithOptions(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create options table
	L := lua.NewState()
	defer L.Close()
	options := L.NewTable()
	options.RawSetString("timeout", lua.LNumber(10))

	headersTable := L.NewTable()
	headersTable.RawSetString("User-Agent", lua.LString("Test-Bot/1.0"))
	headersTable.RawSetString("Accept", lua.LString("application/json"))
	options.RawSetString("headers", headersTable)

	// Test HTTP GET with options
	result, err := engine.httpGet("https://httpbin.org/get", options)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check status
		if status := tbl.RawGetString("status"); status == lua.LNil {
			t.Error("Expected status field")
		} else if statusNum, ok := status.(lua.LNumber); !ok {
			t.Errorf("Expected status to be number, got %T", status)
		} else if statusNum != 200 {
			t.Errorf("Expected status 200, got %v", statusNum)
		}
	}
}

func TestHttpPostBasic(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Test basic HTTP POST
	body := `{"test": "data"}`
	result, err := engine.httpPost("https://httpbin.org/post", body, nil)
	if err != nil {
		t.Fatalf("httpPost failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check status
		if status := tbl.RawGetString("status"); status == lua.LNil {
			t.Error("Expected status field")
		} else if statusNum, ok := status.(lua.LNumber); !ok {
			t.Errorf("Expected status to be number, got %T", status)
		} else if statusNum != 200 {
			t.Errorf("Expected status 200, got %v", statusNum)
		}

		// Check body
		if body := tbl.RawGetString("body"); body == lua.LNil {
			t.Error("Expected body field")
		}
	}
}

func TestHttpPostWithOptions(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create options table
	L := lua.NewState()
	defer L.Close()
	options := L.NewTable()
	options.RawSetString("timeout", lua.LNumber(10))

	headersTable := L.NewTable()
	headersTable.RawSetString("Content-Type", lua.LString("application/json"))
	headersTable.RawSetString("User-Agent", lua.LString("Test-Bot/1.0"))
	options.RawSetString("headers", headersTable)

	// Test HTTP POST with options
	body := `{"message": "test"}`
	result, err := engine.httpPost("https://httpbin.org/post", body, options)
	if err != nil {
		t.Fatalf("httpPost failed: %v", err)
	}

	if result == lua.LNil {
		t.Fatal("Expected result, got nil")
	}

	if tbl, ok := result.(*lua.LTable); !ok {
		t.Errorf("Expected table, got %T", result)
	} else {
		// Check status
		if status := tbl.RawGetString("status"); status == lua.LNil {
			t.Error("Expected status field")
		} else if statusNum, ok := status.(lua.LNumber); !ok {
			t.Errorf("Expected status to be number, got %T", status)
		} else if statusNum != 200 {
			t.Errorf("Expected status 200, got %v", statusNum)
		}
	}
}

func TestHttpGetTimeout(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)

	// Create options table with very short timeout
	L := lua.NewState()
	defer L.Close()
	options := L.NewTable()
	options.RawSetString("timeout", lua.LNumber(0.001)) // 1ms timeout

	// Test HTTP GET with timeout (should fail)
	result, err := engine.httpGet("https://httpbin.org/delay/1", options)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if result != lua.LNil {
		t.Error("Expected nil result on timeout")
	}
}
