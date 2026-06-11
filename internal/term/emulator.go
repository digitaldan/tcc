// Package term wraps the charmbracelet/x/vt terminal emulator for use as an
// embedded, concurrency-safe screen for one Claude Code session.
package term

import (
	"io"
	"strings"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
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
}

func New(w, h int) *Emulator {
	e := &Emulator{
		vt:            vt.NewEmulator(w, h),
		cursorVisible: true,
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
	})
	return e
}

// OnBell registers a callback fired when the child rings the terminal bell.
func (e *Emulator) OnBell(f func()) { e.bell = f }

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
	e.vt.Resize(w, h)
}

func (e *Emulator) Close() { _ = e.vt.Close() }

// View renders the screen as exactly h lines of ANSI-styled text. The cursor
// cell is drawn in reverse video when visible, since the host UI hides the
// real terminal cursor.
func (e *Emulator) View() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	w, h := e.vt.Width(), e.vt.Height()
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
