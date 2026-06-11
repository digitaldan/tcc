// Package claude knows about Claude Code's files and CLI surface: hook
// settings, session transcripts, and background agents.
package claude

import (
	"encoding/json"
	"os"
)

// hookEvents are the events ctmux subscribes to for status tracking.
// PreToolUse/PostToolUse are deliberately omitted: a subprocess per tool call
// buys little — busy is already implied by UserPromptSubmit-without-Stop.
var hookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"Stop",
	"StopFailure",
	"PermissionRequest",
	"Notification",
	"SessionEnd",
}

// WriteHooksSettings writes the settings file passed to claude via
// --settings. Claude Code merges it with the user's own settings, so their
// hooks keep firing alongside ours.
func WriteHooksSettings(path, ctmuxBin string) error {
	type hook struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type matcher struct {
		Hooks []hook `json:"hooks"`
	}

	hooks := map[string][]matcher{}
	for _, ev := range hookEvents {
		hooks[ev] = []matcher{{Hooks: []hook{{Type: "command", Command: ctmuxBin + " _hook"}}}}
	}

	data, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
