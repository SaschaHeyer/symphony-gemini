package workflow

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/symphony-go/symphony/internal/config"
)

// WatchWorkflow watches the workflow file for changes and calls onChange
// with the new WorkflowDefinition and Config when changes are detected.
// Returns a stop function to cease watching.
func WatchWorkflow(path string, onChange func(*WorkflowDefinition, *config.Config)) (func(), error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := watcher.Add(path); err != nil {
		watcher.Close()
		return nil, err
	}

	var (
		stopCh    = make(chan struct{})
		stopped   sync.Once
		debounce  *time.Timer
		debounceMu sync.Mutex
	)

	stopFn := func() {
		stopped.Do(func() {
			close(stopCh)
			watcher.Close()
		})
	}

	go func() {
		for {
			select {
			case <-stopCh:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
					continue
				}

				// Debounce rapid changes (100ms)
				debounceMu.Lock()
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(100*time.Millisecond, func() {
					reloadWorkflow(path, onChange)
				})
				debounceMu.Unlock()

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("workflow watcher error", "error", err)
			}
		}
	}()

	return stopFn, nil
}

func reloadWorkflow(path string, onChange func(*WorkflowDefinition, *config.Config)) {
	wf, err := LoadWorkflow(path)
	if err != nil {
		slog.Error("workflow reload failed: parse error", "error", err, "path", path)
		return
	}

	cfg, err := config.ParseConfig(wf.Config)
	if err != nil {
		slog.Error("workflow reload failed: config parse error", "error", err, "path", path)
		return
	}

	resolved, err := config.ResolveConfig(cfg)
	if err != nil {
		slog.Error("workflow reload failed: config resolve error", "error", err, "path", path)
		return
	}

	slog.Info("workflow reloaded successfully", "path", path)
	onChange(wf, resolved)
}
