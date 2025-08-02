package lua

import (
	"context"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestTimerRegistration(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	timer := NewTimer(engine)

	// Create a test callback
	L := lua.NewState()
	defer L.Close()
	callback := L.NewFunction(func(L *lua.LState) int {
		return 0
	})

	// Register a timer
	timerID := timer.RegisterTimer(1.0, callback, lua.LNil, "test_script.lua")

	if timerID == "" {
		t.Fatal("Expected timer ID, got empty string")
	}

	// Check that timer is active
	if timer.GetTimerCount() != 1 {
		t.Errorf("Expected 1 active timer, got %d", timer.GetTimerCount())
	}

	// Check that timer is in active timers list
	activeTimers := timer.GetActiveTimers()
	if len(activeTimers) != 1 {
		t.Errorf("Expected 1 active timer, got %d", len(activeTimers))
	}
	if activeTimers[0] != timerID {
		t.Errorf("Expected timer ID %s, got %s", timerID, activeTimers[0])
	}
}

func TestTimerUnregistration(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	timer := NewTimer(engine)

	// Create a test callback
	L := lua.NewState()
	defer L.Close()
	callback := L.NewFunction(func(L *lua.LState) int {
		return 0
	})

	// Register a timer
	timerID := timer.RegisterTimer(10.0, callback, lua.LNil, "test_script.lua")

	// Unregister the timer
	success := timer.UnregisterTimer(timerID)
	if !success {
		t.Fatal("Expected successful unregistration")
	}

	// Check that timer is no longer active
	if timer.GetTimerCount() != 0 {
		t.Errorf("Expected 0 active timers, got %d", timer.GetTimerCount())
	}

	// Try to unregister non-existent timer
	success = timer.UnregisterTimer("non_existent")
	if success {
		t.Error("Expected unsuccessful unregistration for non-existent timer")
	}
}

func TestTimerExecution(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	engine.Initialize()

	// Start the engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)

	// Create a test callback that will be called
	callbackExecuted := false
	L := lua.NewState()
	defer L.Close()
	callback := L.NewFunction(func(L *lua.LState) int {
		callbackExecuted = true
		return 0
	})

	// Register a timer with short duration
	_ = engine.timer.RegisterTimer(0.1, callback, lua.LNil, "test_script.lua")

	// Wait for timer to execute
	time.Sleep(200 * time.Millisecond)

	if !callbackExecuted {
		t.Error("Expected callback to be executed")
	}

	// Check that timer was removed after execution
	if engine.timer.GetTimerCount() != 0 {
		t.Errorf("Expected 0 active timers after execution, got %d", engine.timer.GetTimerCount())
	}
}

func TestTimerDataPassing(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	engine.Initialize()

	// Start the engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)

	// Create test data
	L := lua.NewState()
	defer L.Close()
	testData := L.NewTable()
	testData.RawSetString("message", lua.LString("test message"))

	// Create a callback that checks the data
	dataReceived := false
	callback := L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() > 0 {
			if data, ok := L.Get(1).(*lua.LTable); ok {
				if message := data.RawGetString("message"); message.String() == "test message" {
					dataReceived = true
				}
			}
		}
		return 0
	})

	// Register a timer with data
	_ = engine.timer.RegisterTimer(0.1, callback, testData, "test_script.lua")

	// Wait for timer to execute
	time.Sleep(200 * time.Millisecond)

	if !dataReceived {
		t.Error("Expected data to be passed to callback")
	}
}

func TestTimerStopAll(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	timer := NewTimer(engine)

	// Create test callbacks
	L := lua.NewState()
	defer L.Close()
	callback := L.NewFunction(func(L *lua.LState) int {
		return 0
	})

	// Register multiple timers
	timer1 := timer.RegisterTimer(10.0, callback, lua.LNil, "test_script1.lua")
	timer2 := timer.RegisterTimer(10.0, callback, lua.LNil, "test_script2.lua")
	timer3 := timer.RegisterTimer(10.0, callback, lua.LNil, "test_script3.lua")

	// Check that all timers are active
	if timer.GetTimerCount() != 3 {
		t.Errorf("Expected 3 active timers, got %d", timer.GetTimerCount())
	}

	// Stop all timers
	timer.StopAll()

	// Check that all timers are stopped
	if timer.GetTimerCount() != 0 {
		t.Errorf("Expected 0 active timers after StopAll, got %d", timer.GetTimerCount())
	}

	// Verify individual timers are stopped
	if timer.UnregisterTimer(timer1) {
		t.Error("Expected timer1 to already be stopped")
	}
	if timer.UnregisterTimer(timer2) {
		t.Error("Expected timer2 to already be stopped")
	}
	if timer.UnregisterTimer(timer3) {
		t.Error("Expected timer3 to already be stopped")
	}
}

func TestRepeatingTimer(t *testing.T) {
	db := setupTestDB(t)
	engine := New(db, nil)
	engine.Initialize()

	// Start the engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)

	// Create a test callback that will be called multiple times
	executionCount := 0
	L := lua.NewState()
	defer L.Close()
	callback := L.NewFunction(func(L *lua.LState) int {
		executionCount++
		return 0
	})

	// Register a repeating timer with short duration
	timerID := engine.timer.RegisterRepeatingTimer(0.1, callback, lua.LNil, "test_script.lua")

	// Wait for multiple executions
	time.Sleep(500 * time.Millisecond)

	// Should have executed multiple times
	if executionCount < 3 {
		t.Errorf("Expected at least 3 executions, got %d", executionCount)
	}

	// Check that timer is still active (repeating)
	if engine.timer.GetTimerCount() != 1 {
		t.Errorf("Expected 1 active timer (repeating), got %d", engine.timer.GetTimerCount())
	}

	// Cancel the repeating timer
	success := engine.timer.UnregisterTimer(timerID)
	if !success {
		t.Error("Expected successful cancellation of repeating timer")
	}

	// Check that timer is no longer active
	if engine.timer.GetTimerCount() != 0 {
		t.Errorf("Expected 0 active timers after cancellation, got %d", engine.timer.GetTimerCount())
	}
}
