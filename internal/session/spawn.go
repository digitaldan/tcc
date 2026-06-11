package session

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
	ExtraArgs    []string // e.g. ["--resume", "<id>"] or ["attach", "<short>"]
	ClaudeBin    string   // defaults to "claude" from PATH
	PreassignID  bool     // pass --session-id with a fresh UUID
}

// NewClaude builds a Session (not yet started) running claude with ctmux's
// hook wiring: CTMUX_TAB_ID in the environment and --settings for status
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

	args := []string{}
	if opts.SettingsFile != "" {
		args = append(args, "--settings", opts.SettingsFile)
	}
	if opts.PreassignID {
		s.SessionID = NewUUID()
		args = append(args, "--session-id", s.SessionID)
	}
	args = append(args, opts.ExtraArgs...)

	cmd := exec.Command(bin, args...)
	cmd.Dir = opts.Dir
	cmd.Env = append(os.Environ(), "CTMUX_TAB_ID="+s.TabID)
	s.Cmd = cmd

	if s.Title == "" {
		s.Title = filepath.Base(opts.Dir)
	}
	return s
}
