package status

// State is a tab's session state, derived from hook events and process
// lifecycle.
type State int

const (
	Starting State = iota
	Busy
	Idle
	NeedsInput
	Error
	Exited
)

func (s State) String() string {
	switch s {
	case Starting:
		return "starting"
	case Busy:
		return "busy"
	case Idle:
		return "idle"
	case NeedsInput:
		return "needs_input"
	case Error:
		return "error"
	case Exited:
		return "exited"
	}
	return "unknown"
}

// Glyph returns the tab-bar indicator for the state.
func (s State) Glyph() string {
	switch s {
	case Starting:
		return "◌"
	case Busy:
		return "✶"
	case Idle:
		return "○"
	case NeedsInput:
		return "●"
	case Error:
		return "✕"
	case Exited:
		return "▢"
	}
	return "?"
}

// FromHookEvent maps a hook event name to the resulting state. The bool is
// false when the event doesn't change state.
func FromHookEvent(event string) (State, bool) {
	switch event {
	case "SessionStart":
		return Idle, true
	case "UserPromptSubmit":
		return Busy, true
	case "PermissionRequest", "Notification":
		return NeedsInput, true
	case "Stop":
		return Idle, true
	case "StopFailure":
		return Error, true
	case "SessionEnd":
		return Exited, true
	}
	return Starting, false
}
