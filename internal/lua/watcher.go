package lua

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watcher handles file watching for script reloading
type Watcher struct {
	engine *Engine
	dir    string
}

// NewWatcher creates a new file watcher
func NewWatcher(engine *Engine, dir string) *Watcher {
	return &Watcher{
		engine: engine,
		dir:    dir,
	}
}

// Start begins watching for file changes
func (w *Watcher) Start(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("File watcher error:", err)
		return
	}

	go func() {
		defer watcher.Close()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Println("Script watcher closed")
					return
				}

				// Only process .lua files and ignore files starting with '.'
				if !w.shouldProcessFile(event.Name) {
					continue
				}

				// todo: handle removed/deleted files

				log.Println("File watcher event:", event)
				if event.Has(fsnotify.Write) {
					log.Println("Reloading script due to change:", event.Name)
					event := ScriptEvent{
						ScriptName: event.Name,
						Action:     "reload",
					}
					w.engine.enqueueEvent(event, "watcher")
				} else if event.Has(fsnotify.Create) {
					log.Println("New script detected:", event.Name)
					event := ScriptEvent{
						ScriptName: event.Name,
						Action:     "load",
					}
					w.engine.enqueueEvent(event, "watcher")
				}

			case err := <-watcher.Errors:
				if err != nil {
					log.Println("Watcher error:", err)
				}
			case <-ctx.Done():
				log.Println("Stopping file watcher...")
				return
			}
		}
	}()

	if err := watcher.Add(w.dir); err != nil {
		log.Println("Failed to add directory to watcher:", err)
	}
}

// shouldProcessFile checks if a file should be processed by the watcher
func (w *Watcher) shouldProcessFile(filename string) bool {
	base := filepath.Base(filename)
	if !strings.HasPrefix(base, ".") && filepath.Ext(base) == ".lua" {
		// it's a non-hidden .lua file
		return true
	}
	return false
}
