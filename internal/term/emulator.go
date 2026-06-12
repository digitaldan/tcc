// Package term wraps the charmbracelet/x/vt terminal emulator for use as an
// embedded, concurrency-safe screen for one Claude Code session.
package term

import (
	"io"
	"strings"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// Emulator is a concurrency-safe virtual terminal. The PTY reader goroutine
// calls Feed, the UI goroutine calls View/Resize, and Responses must be
// drained into the PTY so the child's capability queries (DA1, CPR, etc.)
// are answered.
type Emulator struct {
	mu            sync.Mutex
	vt            *vt.Emulator
	cursorVisible bool
	bell          func()
	mouseChange   func(enabled bool)
	mouseModes    map[int]bool
	scrollOffset  int // lines scrolled back into history (0 = live)
}

func New(w, h int) *Emulator {
	e := &Emulator{
		vt:            vt.NewEmulator(w, h),
		cursorVisible: true,
		mouseModes:    map[int]bool{},
	}
	e.vt.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) {
			// Called from within Feed; mu is already held.
			e.cursorVisible = visible
		},
		Bell: func() {
			if e.bell != nil {
				e.bell()
			}
		},
		EnableMode:  func(m ansi.Mode) { e.modeChanged(m, true) },
		DisableMode: func(m ansi.Mode) { e.modeChanged(m, false) },
	})
	return e
}

// modeChanged tracks the child's mouse-tracking modes so the host can mirror
// them onto the real terminal. Called from within Feed; mu is held.
func (e *Emulator) modeChanged(m ansi.Mode, on bool) {
	dec, ok := m.(ansi.DECMode)
	if !ok {
		return
	}
	switch int(dec) {
	case 9, 1000, 1002, 1003:
		before := e.mouseWanted()
		e.mouseModes[int(dec)] = on
		if after := e.mouseWanted(); after != before && e.mouseChange != nil {
			e.mouseChange(after)
		}
	}
}

func (e *Emulator) mouseWanted() bool {
	for _, on := range e.mouseModes {
		if on {
			return true
		}
	}
	return false
}

// MouseWanted reports whether the child currently has any mouse tracking
// mode enabled.
func (e *Emulator) MouseWanted() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mouseWanted()
}

// MouseLevel returns the highest mouse-tracking mode the child has enabled
// (9, 1000, 1002, or 1003), or 0 when tracking is off. Used to decide which
// event kinds to forward.
func (e *Emulator) MouseLevel() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	level := 0
	for mode, on := range e.mouseModes {
		if on && mode > level {
			level = mode
		}
	}
	return level
}

// OnBell registers a callback fired when the child rings the terminal bell.
func (e *Emulator) OnBell(f func()) { e.bell = f }

// OnMouseChange registers a callback fired when the child enables or
// disables mouse tracking.
func (e *Emulator) OnMouseChange(f func(enabled bool)) { e.mouseChange = f }

// Feed writes child output into the emulator.
func (e *Emulator) Feed(p []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = e.vt.Write(p)
}

// Responses returns the reader carrying bytes the emulator wants sent back to
// the child (answers to terminal queries). It blocks until data is available
// and returns EOF after Close.
func (e *Emulator) Responses() io.Reader { return e.vt }

func (e *Emulator) Resize(w, h int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scrollOffset = 0
	e.vt.Resize(w, h)
}

// ScrollBy moves the view into scrollback history (positive = older lines)
// and reports whether the offset changed. Scrolling is a no-op on the alt
// screen — full-screen apps handle their own scrolling.
func (e *Emulator) ScrollBy(delta int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.vt.IsAltScreen() {
		return false
	}
	off := e.scrollOffset + delta
	if max := e.vt.ScrollbackLen(); off > max {
		off = max
	}
	if off < 0 {
		off = 0
	}
	changed := off != e.scrollOffset
	e.scrollOffset = off
	return changed
}

// ResetScroll jumps back to the live view.
func (e *Emulator) ResetScroll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scrollOffset = 0
}

// ScrollPosition returns the current offset into history and the total
// history size. offset == 0 means the live view.
func (e *Emulator) ScrollPosition() (offset, total int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scrollOffset, e.vt.ScrollbackLen()
}

func (e *Emulator) Close() { _ = e.vt.Close() }

// View renders the screen as exactly h lines of ANSI-styled text. The cursor
// cell is drawn in reverse video when visible, since the host UI hides the
// real terminal cursor. When scrolled back, the view is composed of
// scrollback lines followed by the top of the live screen.
func (e *Emulator) View() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	w, h := e.vt.Width(), e.vt.Height()

	if e.scrollOffset > 0 {
		return e.renderScrolled(h)
	}

	cur := e.vt.CursorPosition()

	lines := strings.Split(e.vt.Render(), "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}

	if e.cursorVisible && cur.Y >= 0 && cur.Y < h {
		lines[cur.Y] = e.renderLineWithCursor(cur.Y, cur.X, w)
	}
	return strings.Join(lines, "\n")
}

// renderScrolled composes the view at the current scrollback offset: the
// last offset lines of history on top, then the upper part of the live
// screen. Must be called with mu held.
func (e *Emulator) renderScrolled(h int) string {
	total := e.vt.ScrollbackLen()
	off := e.scrollOffset
	if off > total {
		off = total
	}

	lines := make([]string, 0, h)
	// History portion: the off oldest-of-the-recent lines, ending at the
	// line that immediately precedes the live screen.
	for i := total - off; i < total && len(lines) < h; i++ {
		lines = append(lines, e.renderScrollbackLine(i))
	}
	// Live screen portion fills the remainder from its top.
	screen := strings.Split(e.vt.Render(), "\n")
	for i := 0; i < len(screen) && len(lines) < h; i++ {
		lines = append(lines, screen[i])
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderScrollbackLine renders one history line. Must be called with mu held.
func (e *Emulator) renderScrollbackLine(idx int) string {
	if line := e.vt.Scrollback().Line(idx); line != nil {
		return line.Render()
	}
	return ""
}

// renderLineWithCursor rebuilds one line from cells, toggling reverse video
// on the cursor cell. Must be called with mu held.
func (e *Emulator) renderLineWithCursor(y, cx, w int) string {
	line := make(uv.Line, w)
	for x := 0; x < w; x++ {
		if c := e.vt.CellAt(x, y); c != nil {
			line[x] = *c
		} else {
			line[x] = uv.EmptyCell
		}
	}
	if cx >= 0 && cx < w {
		c := line[cx]
		if c.IsZero() || c.Content == "" {
			c = uv.EmptyCell
			c.Content = " "
		}
		c.Style.Attrs ^= uv.AttrReverse
		line[cx] = c
	}
	return line.Render()
}
