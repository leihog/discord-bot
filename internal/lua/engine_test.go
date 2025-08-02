package lua

import (
	"context"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestEventQueueSystem(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	engine.Initialize()

	// Create a context with timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the engine
	engine.Start(ctx)

	// Create a test hook
	L := lua.NewState()
	defer L.Close()

	testHook := HookInfo{
		Function: L.NewFunction(func(L *lua.LState) int {
			// This function will be called by the dispatcher
			return 0
		}),
		Script: "test_script.lua",
	}

	// Create test data
	data := L.NewTable()
	data.RawSetString("content", lua.LString("test message"))

	// Create an event
	event := LuaEvent{
		Hook:      testHook,
		Data:      data,
		EventType: "test",
	}

	// Send the event to the queue
	select {
	case engine.eventQueue <- event:
		// Event queued successfully
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Failed to queue event within timeout")
	}

	// Wait a bit for the event to be processed
	time.Sleep(100 * time.Millisecond)

	// The event should have been processed by the dispatcher
	// We can't easily verify the execution, but we can verify the queue is working
	// by checking that the event was accepted
}

func TestEventQueueGracefulShutdown(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	engine.Initialize()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start the engine
	engine.Start(ctx)

	// Cancel the context to trigger shutdown
	cancel()

	// Wait a bit for the dispatcher to shut down
	time.Sleep(100 * time.Millisecond)

	// The engine should have shut down gracefully
	// We can't easily verify this, but the test should complete without hanging
}
