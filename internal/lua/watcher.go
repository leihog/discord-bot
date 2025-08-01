package lua

import (
	"context"
	"log"
	"path/filepath"

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
	defer watcher.Close()

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if filepath.Ext(event.Name) == ".lua" && (event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0) {
					log.Println("Reloading scripts due to change:", event.Name)
					w.engine.LoadScripts(w.dir)
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
