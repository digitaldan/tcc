package app

import "github.com/dcunningham/ctmux/internal/status"

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
