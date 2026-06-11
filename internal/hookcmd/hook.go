// Package hookcmd implements `tcc _hook`, the command Claude Code invokes
// on session events. It must be fast (it runs on every prompt/stop) and must
// always exit 0 so it never blocks Claude.
package hookcmd

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/digitaldan/tcc/internal/config"
	"github.com/digitaldan/tcc/internal/status"
)

// payload is the subset of Claude Code's hook stdin JSON that tcc uses.
type payload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	Message        string `json:"message"` // Notification hooks
}

// Run reads the hook payload from stdin and records it in the per-tab state
// file. Errors are deliberately swallowed: a broken status badge is better
// than a hook failure surfacing inside Claude.
func Run() {
	tabID := os.Getenv("TCC_TAB_ID")
	if tabID == "" {
		return // claude run outside tcc with our settings file; ignore
	}

	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return
	}
	var p payload
	if json.Unmarshal(data, &p) != nil {
		return
	}

	dir, err := config.StateDir()
	if err != nil {
		return
	}
	_ = status.WriteHookEvent(dir, status.HookEvent{
		TabID:          tabID,
		SessionID:      p.SessionID,
		Event:          p.HookEventName,
		TS:             time.Now().UTC().Format(time.RFC3339Nano),
		CWD:            p.CWD,
		TranscriptPath: p.TranscriptPath,
		Message:        p.Message,
	})
}
