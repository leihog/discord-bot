package lua

import (
	"log"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// TimerEntry represents a single timer
type TimerEntry struct {
	ID        string
	Duration  time.Duration
	Callback  lua.LValue
	Data      lua.LValue
	Script    *LuaScript
	Timer     *time.Timer
	Active    bool
	Repeating bool
}

// Timer manages Lua script timers
type Timer struct {
	timers map[string]*TimerEntry
	mu     sync.RWMutex
	engine *Engine
}

// NewTimer creates a new timer manager
func NewTimer(engine *Engine) *Timer {
	return &Timer{
		timers: make(map[string]*TimerEntry),
		engine: engine,
	}
}

// RegisterTimer registers a new timer
func (t *Timer) RegisterTimer(seconds float64, callback lua.LValue, data lua.LValue, script *LuaScript) string {
	return t.registerTimer(seconds, callback, data, script, false)
}

// RegisterRepeatingTimer registers a new repeating timer
func (t *Timer) RegisterRepeatingTimer(seconds float64, callback lua.LValue, data lua.LValue, script *LuaScript) string {
	return t.registerTimer(seconds, callback, data, script, true)
}

// registerTimer registers a new timer (internal function)
func (t *Timer) registerTimer(seconds float64, callback lua.LValue, data lua.LValue, script *LuaScript, repeating bool) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Generate unique timer ID
	timerID := generateTimerID()
	duration := time.Duration(seconds * float64(time.Second))

	// Create timer entry
	entry := &TimerEntry{
		ID:        timerID,
		Duration:  duration,
		Callback:  callback,
		Data:      data,
		Script:    script,
		Active:    true,
		Repeating: repeating,
	}

	// Create the actual timer
	entry.Timer = time.AfterFunc(duration, func() {
		t.executeTimer(timerID)
	})

	// Store the timer
	t.timers[timerID] = entry

	timerType := "one-shot"
	if repeating {
		timerType = "repeating"
	}
	log.Printf("Registered %s timer '%s' for script '%s' (%.2f seconds)", timerType, timerID, script.Name, seconds)
	return timerID
}

// UnregisterTimer cancels and removes a timer
func (t *Timer) UnregisterTimer(timerID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, exists := t.timers[timerID]
	if !exists {
		return false
	}

	// Stop the timer if it's still active
	if entry.Active && entry.Timer != nil {
		entry.Timer.Stop()
		entry.Active = false
	}

	// Remove from map
	delete(t.timers, timerID)

	log.Printf("Unregistered timer '%s' from script '%s'", timerID, entry.Script.Name)
	return true
}

// Removes any pending timers registered by a script
func (t *Timer) UnregisterScriptTimers(scriptName string) {
	// it's necessary to fetch the timers in a separate lock to avoid deadlocks
	t.mu.Lock()
	var targetTimers []string
	for timerID, entry := range t.timers {
		if entry.Script.Name == scriptName {
			targetTimers = append(targetTimers, timerID)
		}
	}
	t.mu.Unlock()

	for _, timerID := range targetTimers {
		t.UnregisterTimer(timerID)
	}
}

// executeTimer executes a timer callback
func (t *Timer) executeTimer(timerID string) {
	t.mu.RLock()
	entry, exists := t.timers[timerID]
	if !exists {
		t.mu.RUnlock()
		return
	}
	t.mu.RUnlock()

	// Mark as inactive
	t.mu.Lock()
	entry.Active = false
	t.mu.Unlock()

	// Create event for the timer callback
	event := TimerEvent{
		TimerID: timerID,
		Callback: HookInfo{
			Function: entry.Callback,
			Script:   entry.Script,
		},
		TimerData: entry.Data,
	}

	// Enqueue the timer event
	select {
	case t.engine.eventQueue <- event:
		log.Printf("Timer '%s' from script '%s' executed", timerID, entry.Script.Name)
	default:
		log.Printf("Warning: Could not enqueue timer '%s' from script '%s' - queue full", timerID, entry.Script.Name)
	}

	// Handle repeating timers
	if entry.Repeating {
		t.mu.Lock()
		// Re-register the timer for the next execution
		entry.Timer = time.AfterFunc(entry.Duration, func() {
			t.executeTimer(timerID)
		})
		entry.Active = true
		t.mu.Unlock()
		log.Printf("Re-registered repeating timer '%s' from script '%s'", timerID, entry.Script.Name)
	} else {
		// Remove the timer from the map since it's completed (one-shot)
		t.mu.Lock()
		delete(t.timers, timerID)
		t.mu.Unlock()
	}
}

// GetActiveTimers returns a list of active timer IDs
func (t *Timer) GetActiveTimers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var activeTimers []string
	for timerID, entry := range t.timers {
		if entry.Active {
			activeTimers = append(activeTimers, timerID)
		}
	}
	return activeTimers
}

// GetTimerCount returns the number of active timers
func (t *Timer) GetTimerCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, entry := range t.timers {
		if entry.Active {
			count++
		}
	}
	return count
}

// StopAll stops all active timers
func (t *Timer) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for timerID, entry := range t.timers {
		if entry.Active && entry.Timer != nil {
			entry.Timer.Stop()
			entry.Active = false
			log.Printf("Stopped timer '%s' from script '%s'", timerID, entry.Script.Name)
		}
		// Remove from map
		delete(t.timers, timerID)
	}
}

// generateTimerID generates a unique timer ID
func generateTimerID() string {
	return "timer_" + time.Now().Format("20060102150405.000000000")
}
