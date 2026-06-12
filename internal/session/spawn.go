package session

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cleanEnv returns the environment without Claude Code's nested-session
// markers. If tcc itself was launched from inside a Claude Code session,
// children inheriting CLAUDECODE / CLAUDE_CODE_* believe they are child
// sessions and silently skip writing conversation transcripts — which would
// break resume and tab restore. CLAUDE_CONFIG_DIR is kept (user intent).
func cleanEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// NewUUID returns a random v4 UUID string.
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// SpawnOptions configures a new claude child process.
type SpawnOptions struct {
	Dir          string   // working directory
	SettingsFile string   // hooks settings file passed via --settings ("" to skip)
	ExtraArgs    []string // e.g. ["--resume", "<id>"]
	ClaudeBin    string   // defaults to "claude" from PATH
	PreassignID  bool     // pass --session-id with a fresh UUID
	Attach       string   // daemon short id; runs `claude attach <short>` (no settings/session-id)
}

// NewClaude builds a Session (not yet started) running claude with tcc's
// hook wiring: TCC_TAB_ID in the environment and --settings for status
// hooks. Call Start on the result.
func NewClaude(opts SpawnOptions) *Session {
	bin := opts.ClaudeBin
	if bin == "" {
		bin = "claude"
	}

	s := &Session{
		TabID: NewUUID(),
		Dir:   opts.Dir,
		Kind:  KindSpawned,
	}

	var args []string
	if opts.Attach != "" {
		// `claude attach` joins a daemon worker; per-session settings flags
		// don't apply to the already-running worker.
		args = []string{"attach", opts.Attach}
		s.Kind = KindAttached
		s.AgentShort = opts.Attach
	} else {
		if opts.SettingsFile != "" {
			args = append(args, "--settings", opts.SettingsFile)
		}
		if opts.PreassignID {
			s.SessionID = NewUUID()
			args = append(args, "--session-id", s.SessionID)
		}
		args = append(args, opts.ExtraArgs...)
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = opts.Dir
	cmd.Env = append(cleanEnv(), "TCC_TAB_ID="+s.TabID)
	s.Cmd = cmd

	if s.Title == "" {
		s.Title = filepath.Base(opts.Dir)
	}
	return s
}
