package app

import (
	"github.com/digitaldan/ctmux/internal/claude"
	"github.com/digitaldan/ctmux/internal/status"
)

// damageMsg signals that a session's screen changed.
type damageMsg struct{ tabID string }

// sessionExitMsg signals that a session's child process exited.
type sessionExitMsg struct {
	tabID string
	code  int
}

// prefixMsg signals that the prefix key (Ctrl+Q) was pressed in session mode.
type prefixMsg struct{}

// hookEventMsg carries a Claude Code hook event from the state-dir watcher.
type hookEventMsg struct{ ev status.HookEvent }

// tickMsg drives periodic UI refresh (busy spinner animation).
type tickMsg struct{}

// jobStateMsg carries a daemon job-state change for an attached tab.
type jobStateMsg struct {
	tabID string
	js    claude.JobState
}

// bellMsg signals that a session rang the terminal bell.
type bellMsg struct{ tabID string }

// tabClickMsg reports a mouse press on the tab bar (1-based column).
type tabClickMsg struct{ col int }

// tabNavMsg reports a Ctrl+Shift+Left/Right tab switch (-1 / +1).
type tabNavMsg struct{ delta int }
