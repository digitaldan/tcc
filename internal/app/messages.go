package app

// damageMsg signals that a session's screen changed.
type damageMsg struct{ tabID string }

// sessionExitMsg signals that a session's child process exited.
type sessionExitMsg struct {
	tabID string
	code  int
}

// prefixMsg signals that the prefix key (Ctrl+Q) was pressed in session mode.
type prefixMsg struct{}
