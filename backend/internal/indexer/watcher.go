package indexer

import (
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// Watcher continuously monitors a directory for file changes.
type Watcher struct {
	watcher *fsnotify.Watcher
}

// NewWatcher creates and returns a new Watcher.
func NewWatcher() (*Watcher, error) {
	fsnWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{watcher: fsnWatcher}, nil
}

// Start begins watching the specified root directory.
func (w *Watcher) Start(root string) (<-chan fsnotify.Event, error) {
	// Walk the directory and add all subdirectories to the watcher.
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return w.watcher.Add(path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	log.Printf("👀 Started watching directory: %s", root)

	// Goroutine to handle events and errors
	go func() {
		defer w.watcher.Close()
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				log.Printf("✨ File event: %s %s", event.Name, event.Op)
				// Here we would trigger re-indexing for the changed file.
			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("🔥 Watcher error: %v", err)
			}
		}
	}()

	return w.watcher.Events, nil
}
