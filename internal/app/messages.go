package app

import (
	"github.com/digitaldan/tcc/internal/claude"
	"github.com/digitaldan/tcc/internal/status"
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

// tabTitleMsg carries a terminal tab's shell-set title (OSC 0/2).
type tabTitleMsg struct {
	tabID string
	title string
}

// tabCwdMsg carries a terminal tab's reported working directory (OSC 7).
type tabCwdMsg struct {
	tabID string
	dir   string
}

// tabClickMsg reports a mouse press on the tab bar (1-based column).
type tabClickMsg struct{ col int }

// tabNavMsg reports a Ctrl+Shift+Left/Right tab switch (-1 / +1).
type tabNavMsg struct{ delta int }

// wheelMsg reports wheel input the session didn't consume (scrollback).
type wheelMsg struct{ delta int }

// pageScrollMsg reports a Ctrl+PageUp/PageDown page-scroll of scrollback
// (-1 up / +1 down).
type pageScrollMsg struct{ delta int }

// scrollResetMsg snaps the active tab back to the live view.
type scrollResetMsg struct{}

// pickerRefreshMsg asks the app to rebuild a picker after a destructive
// action (session deleted, agent stopped/removed) completed.
type pickerRefreshMsg struct{ mode uiMode }

// agentStoppedMsg signals that a background worker was stopped and its
// session can now be resumed interactively with full history.
type agentStoppedMsg struct {
	sessionID string
	dir       string
	title     string
}
