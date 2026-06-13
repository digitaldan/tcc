package term

import (
	"bytes"
	"testing"
)

type sink struct{ bytes.Buffer }

func TestFilterSessionPassthroughAndPrefix(t *testing.T) {
	r := NewRouter(0)
	var clicked int
	r.OnTabClick(func(col int) { clicked = col })

	// Plain text + ESC key pass through untouched.
	out, rest := r.filterSession([]byte("hello\x1bworld"))
	if string(out) != "hello\x1bworld" || rest != nil {
		t.Fatalf("passthrough broken: %q rest=%q", out, rest)
	}

	// Prefix splits the stream and switches mode.
	r.SetMode(ModeSession)
	out, rest = r.filterSession([]byte("ab\x11cd"))
	if string(out) != "ab" || string(rest) != "cd" {
		t.Fatalf("prefix split broken: out=%q rest=%q", out, rest)
	}
	if r.Mode() != ModeChrome {
		t.Fatal("prefix did not switch to chrome mode")
	}

	// SGR mouse press at row 5 is shifted to row 4 (session has mouse on).
	r.SetMouseLevel(1003)
	out, _ = r.filterSession([]byte("\x1b[<0;10;5M"))
	if string(out) != "\x1b[<0;10;4M" {
		t.Fatalf("mouse rewrite broken: %q", out)
	}

	// Press on the tab bar (row 1) is intercepted as a click.
	out, _ = r.filterSession([]byte("\x1b[<0;7;1M"))
	if len(out) != 0 || clicked != 7 {
		t.Fatalf("tab click broken: out=%q clicked=%d", out, clicked)
	}

	// Wheel on the tab bar is dropped, not clicked.
	clicked = 0
	out, _ = r.filterSession([]byte("\x1b[<64;7;1M"))
	if len(out) != 0 || clicked != 0 {
		t.Fatalf("tab wheel handling broken: out=%q clicked=%d", out, clicked)
	}

	// Arrow keys and other CSI pass through.
	out, _ = r.filterSession([]byte("\x1b[A\x1b[1;2B"))
	if string(out) != "\x1b[A\x1b[1;2B" {
		t.Fatalf("CSI passthrough broken: %q", out)
	}
}

func TestFilterSessionTabNavShortcut(t *testing.T) {
	r := NewRouter(0)
	var nav int
	r.OnTabNav(func(d int) { nav += d })

	// Ctrl+Shift+Right / Left are intercepted, surrounding bytes forwarded.
	out, _ := r.filterSession([]byte("a\x1b[1;6Cb\x1b[1;6Dc"))
	if string(out) != "abc" || nav != 0 {
		t.Fatalf("nav interception broken: out=%q nav=%d", out, nav)
	}

	out, _ = r.filterSession([]byte("\x1b[1;6C\x1b[1;6C"))
	if len(out) != 0 || nav != 2 {
		t.Fatalf("nav delta broken: out=%q nav=%d", out, nav)
	}

	// Plain Ctrl+Right (modifier 5) is NOT intercepted.
	out, _ = r.filterSession([]byte("\x1b[1;5C"))
	if string(out) != "\x1b[1;5C" {
		t.Fatalf("ctrl+right should pass through: %q", out)
	}

	// Partial nav candidate at the read boundary is carried.
	out, _ = r.filterSession([]byte("x\x1b[1;6"))
	if string(out) != "x" || len(r.carry) == 0 {
		t.Fatalf("nav carry broken: out=%q carry=%q", out, r.carry)
	}
}

func TestMouseLevelGating(t *testing.T) {
	r := NewRouter(0)

	press := []byte("\x1b[<0;10;5M")
	hover := []byte("\x1b[<35;10;5M") // motion, no button
	drag := []byte("\x1b[<32;10;5M")  // motion, left button held

	// Level 0: everything for the session is dropped.
	out, _ := r.filterSession(press)
	if len(out) != 0 {
		t.Fatalf("level 0 should drop presses: %q", out)
	}

	// Level 1000: presses pass (row-shifted), motion dropped.
	r.SetMouseLevel(1000)
	out, _ = r.filterSession(press)
	if string(out) != "\x1b[<0;10;4M" {
		t.Fatalf("level 1000 press broken: %q", out)
	}
	out, _ = r.filterSession(hover)
	if len(out) != 0 {
		t.Fatalf("level 1000 should drop hover: %q", out)
	}
	out, _ = r.filterSession(drag)
	if len(out) != 0 {
		t.Fatalf("level 1000 should drop drags: %q", out)
	}

	// Level 1002: drags pass, hover still dropped.
	r.SetMouseLevel(1002)
	out, _ = r.filterSession(drag)
	if string(out) != "\x1b[<32;10;4M" {
		t.Fatalf("level 1002 drag broken: %q", out)
	}
	out, _ = r.filterSession(hover)
	if len(out) != 0 {
		t.Fatalf("level 1002 should drop hover: %q", out)
	}

	// Level 1003: everything passes.
	r.SetMouseLevel(1003)
	out, _ = r.filterSession(hover)
	if string(out) != "\x1b[<35;10;4M" {
		t.Fatalf("level 1003 hover broken: %q", out)
	}
}

func TestFilterSessionCarriesSplitMouseSequence(t *testing.T) {
	r := NewRouter(0)

	out, rest := r.filterSession([]byte("x\x1b[<0;10"))
	if string(out) != "x" || rest != nil {
		t.Fatalf("split handling broken: out=%q rest=%q", out, rest)
	}
	if len(r.carry) == 0 {
		t.Fatal("expected carry for split mouse sequence")
	}

	// A lone trailing ESC must NOT be carried (ESC key responsiveness).
	r.carry = nil
	out, _ = r.filterSession([]byte("y\x1b"))
	if string(out) != "y\x1b" || len(r.carry) != 0 {
		t.Fatalf("lone ESC mishandled: out=%q carry=%q", out, r.carry)
	}
}

// Wheel events the child didn't subscribe to go to the scrollback callback
// instead of being forwarded or dropped.
func TestWheelFallbackToScrollback(t *testing.T) {
	r := NewRouter(0)
	var wheel int
	r.OnWheel(func(d int) { wheel += d })

	// Mouse level 0: wheel-up triggers the callback, nothing is forwarded.
	out, _ := r.filterSession([]byte("\x1b[<64;10;5M"))
	if len(out) != 0 || wheel != -1 {
		t.Fatalf("wheel fallback broken: out=%q wheel=%d", out, wheel)
	}
	out, _ = r.filterSession([]byte("\x1b[<65;10;5M"))
	if len(out) != 0 || wheel != 0 {
		t.Fatalf("wheel-down fallback broken: out=%q wheel=%d", out, wheel)
	}

	// With mouse enabled, wheel forwards to the child instead.
	r.SetMouseLevel(1003)
	out, _ = r.filterSession([]byte("\x1b[<64;10;5M"))
	if string(out) != "\x1b[<64;10;4M" || wheel != 0 {
		t.Fatalf("wheel should forward when child wants mouse: out=%q wheel=%d", out, wheel)
	}

	// Keystrokes while scrolled trigger the snap-back callback.
	var snapped bool
	r.OnAnyKey(func() { snapped = true })
	r.SetScrolled(true)
	_, _ = r.filterSession([]byte("a"))
	if !snapped {
		t.Fatal("keystroke while scrolled should trigger OnAnyKey")
	}
}

// Ctrl+Shift+Up/Down scroll tcc's buffer like wheel notches.
func TestKeyboardScrollShortcut(t *testing.T) {
	r := NewRouter(0)
	var wheel int
	r.OnWheel(func(d int) { wheel += d })

	out, _ := r.filterSession([]byte("a\x1b[1;6Ab\x1b[1;6Bc"))
	if string(out) != "abc" {
		t.Fatalf("scroll keys leaked to session: %q", out)
	}
	if wheel != 0 {
		t.Fatalf("up+down should cancel out, got %d", wheel)
	}

	wheel = 0
	_, _ = r.filterSession([]byte("\x1b[1;6A\x1b[1;6A"))
	if wheel != -2 {
		t.Fatalf("two ups should give -2, got %d", wheel)
	}

	// Plain Ctrl+Up (modifier 5) passes through to the session.
	out, _ = r.filterSession([]byte("\x1b[1;5A"))
	if string(out) != "\x1b[1;5A" {
		t.Fatalf("ctrl+up should pass through: %q", out)
	}
}

func TestPageScrollShortcut(t *testing.T) {
	r := NewRouter(0)
	var page int
	r.OnPage(func(d int) { page += d })

	// Ctrl+Shift+PageUp/Down are intercepted; surrounding bytes pass through.
	out, _ := r.filterSession([]byte("a\x1b[5;6~b\x1b[6;6~c"))
	if string(out) != "abc" {
		t.Fatalf("page keys leaked to session: %q", out)
	}
	if page != 0 {
		t.Fatalf("up+down should cancel out, got %d", page)
	}

	page = 0
	_, _ = r.filterSession([]byte("\x1b[5;6~\x1b[5;6~"))
	if page != -2 {
		t.Fatalf("two page-ups should give -2, got %d", page)
	}

	// Bare PageUp/PageDown (no modifiers) pass through to the session.
	out, _ = r.filterSession([]byte("\x1b[5~\x1b[6~"))
	if string(out) != "\x1b[5~\x1b[6~" {
		t.Fatalf("bare page keys should pass through: %q", out)
	}

	// Plain Ctrl+PageUp/Down (modifier 5) pass through — many terminals bind
	// those to their own tab switching, so we use Ctrl+Shift instead.
	out, _ = r.filterSession([]byte("\x1b[5;5~\x1b[6;5~"))
	if string(out) != "\x1b[5;5~\x1b[6;5~" {
		t.Fatalf("ctrl+page keys should pass through: %q", out)
	}

	// A partial Ctrl+Shift+PageUp at the read boundary is carried, not leaked.
	r.carry = nil
	out, _ = r.filterSession([]byte("z\x1b[5;6"))
	if string(out) != "z" || len(r.carry) == 0 {
		t.Fatalf("page carry broken: out=%q carry=%q", out, r.carry)
	}
}
