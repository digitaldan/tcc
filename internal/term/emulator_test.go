package term

import (
	"strings"
	"testing"
	"time"
)

// Regression test for the 0x9C-in-OSC bug: a title containing U+2733 (whose
// UTF-8 encoding includes the raw C1 ST byte 0x9C) must not leak into the
// screen. See ansipatch.go.
func TestOSCTitleWithUTF8DoesNotLeak(t *testing.T) {
	e := New(40, 4)
	defer e.Close()

	// Byte-by-byte, like a slow PTY read.
	seq := []byte("\x1b]0;✳ Claude Code\x07X")
	for i := range seq {
		e.Feed(seq[i : i+1])
	}

	view := e.View()
	if strings.Contains(view, "Claude") {
		t.Fatalf("OSC title leaked into screen:\n%s", view)
	}
	if !strings.Contains(view, "X") {
		t.Fatalf("expected printable X on screen:\n%s", view)
	}
}

// The emulator must answer Primary Device Attributes queries so children
// like Claude Code don't stall waiting for a terminal response.
func TestRespondsToDA1(t *testing.T) {
	e := New(40, 4)
	defer e.Close()

	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := e.Responses().Read(buf)
		got <- buf[:n]
	}()

	// Feed in a goroutine: the response pipe write blocks until read.
	go e.Feed([]byte("\x1b[c")) // DA1 query

	select {
	case resp := <-got:
		if !strings.HasPrefix(string(resp), "\x1b[?") {
			t.Fatalf("unexpected DA1 response: %q", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no DA1 response from emulator")
	}
}

// Scrollback: feeding more lines than the screen holds pushes history; the
// view composes history + live screen when scrolled, and resets cleanly.
func TestScrollbackView(t *testing.T) {
	e := New(20, 4)
	defer e.Close()

	for i := 1; i <= 12; i++ {
		e.Feed([]byte("line-" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "\r\n"))
	}

	if off, total := e.ScrollPosition(); off != 0 || total == 0 {
		t.Fatalf("expected live view with history, got off=%d total=%d", off, total)
	}

	if !e.ScrollBy(3) {
		t.Fatal("ScrollBy should report a change")
	}
	view := e.View()
	if !strings.Contains(view, "line-07") || strings.Contains(view, "line-12") {
		t.Fatalf("scrolled view should show older lines and hide the newest:\n%s", view)
	}

	// Clamped at history size.
	e.ScrollBy(10000)
	off, total := e.ScrollPosition()
	if off != total {
		t.Fatalf("offset should clamp to total: off=%d total=%d", off, total)
	}
	if !strings.Contains(e.View(), "line-01") {
		t.Fatalf("fully scrolled view should show the oldest line:\n%s", e.View())
	}

	e.ResetScroll()
	if off, _ := e.ScrollPosition(); off != 0 {
		t.Fatal("ResetScroll should return to live view")
	}
	if !strings.Contains(e.View(), "line-12") {
		t.Fatalf("live view should show the newest line:\n%s", e.View())
	}
}
