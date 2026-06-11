package status

import (
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watch monitors the state directory and invokes onEvent for every hook
// event written there. It returns a stop function. Events for unknown tab
// IDs are delivered too; the app filters.
func Watch(dir string, onEvent func(HookEvent)) (func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !ev.Has(fsnotify.Create) && !ev.Has(fsnotify.Write) && !ev.Has(fsnotify.Rename) {
					continue
				}
				name := filepath.Base(ev.Name)
				if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
					continue
				}
				he, err := ReadHookEvent(ev.Name)
				if err != nil || he.TabID == "" {
					continue
				}
				onEvent(he)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return func() { w.Close() }, nil
}
