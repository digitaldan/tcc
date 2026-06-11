package status

import (
	"testing"
	"time"
)

func TestFromHookEvent(t *testing.T) {
	cases := []struct {
		event string
		want  State
		ok    bool
	}{
		{"SessionStart", Idle, true},
		{"UserPromptSubmit", Busy, true},
		{"PermissionRequest", NeedsInput, true},
		{"Notification", NeedsInput, true},
		{"Stop", Idle, true},
		{"StopFailure", Error, true},
		{"SessionEnd", Exited, true},
		{"PreToolUse", Starting, false},
		{"", Starting, false},
	}
	for _, c := range cases {
		got, ok := FromHookEvent(c.event)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("FromHookEvent(%q) = %v,%v want %v,%v", c.event, got, ok, c.want, c.ok)
		}
	}
}

// End-to-end through the file layer: write like the hook command does, watch
// like the app does.
func TestWatchDeliversHookEvents(t *testing.T) {
	dir := t.TempDir()

	got := make(chan HookEvent, 1)
	stop, err := Watch(dir, func(ev HookEvent) { got <- ev })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	want := HookEvent{
		TabID:     "tab-123",
		SessionID: "sess-456",
		Event:     "PermissionRequest",
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := WriteHookEvent(dir, want); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-got:
		if ev.TabID != want.TabID || ev.Event != want.Event || ev.SessionID != want.SessionID {
			t.Fatalf("got %+v want %+v", ev, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher delivered nothing")
	}
}
