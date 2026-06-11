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

	// SGR mouse press at row 5 is shifted to row 4.
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
