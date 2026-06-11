// Package status turns Claude Code hook events into per-tab session states.
package status

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HookEvent is one hook invocation, as persisted to a per-tab state file by
// `tcc _hook` and consumed by the watcher.
type HookEvent struct {
	TabID          string `json:"tab_id"`
	SessionID      string `json:"session_id"`
	Event          string `json:"event"`
	TS             string `json:"ts"`
	CWD            string `json:"cwd,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	Message        string `json:"message,omitempty"`
}

// WriteHookEvent atomically writes the event to <dir>/<tabID>.json.
func WriteHookEvent(dir string, ev HookEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".hook-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, ev.TabID+".json"))
}

// ReadHookEvent reads a state file written by WriteHookEvent.
func ReadHookEvent(path string) (HookEvent, error) {
	var ev HookEvent
	data, err := os.ReadFile(path)
	if err != nil {
		return ev, err
	}
	err = json.Unmarshal(data, &ev)
	return ev, err
}
