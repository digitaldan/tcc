package term

import (
	"bytes"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// Mode selects where raw stdin bytes are routed.
type Mode int32

const (
	// ModeSession forwards stdin bytes verbatim to the active session PTY.
	ModeSession Mode = iota
	// ModeChrome delivers stdin bytes to the TUI (bubbletea) for parsing.
	ModeChrome
)

// DefaultPrefix is the tcc prefix key: Ctrl+Q (DC1, 0x11). Safe because raw
// mode disables IXON flow control and Claude Code does not bind Ctrl+Q.
const DefaultPrefix = 0x11

// Router owns stdin. In session mode bytes go straight to the active PTY so
// nothing is lost in re-encoding (paste blobs, modifiers, control chars). The
// prefix byte flips to chrome mode, where bytes flow to bubbletea instead.
// SGR mouse reports are rewritten down one row to account for the tab bar;
// clicks on the tab bar row are intercepted. Router implements io.Reader for
// use with tea.WithInput.
type Router struct {
	mu     sync.Mutex
	mode   Mode
	active io.Writer

	toTea      chan []byte
	pending    []byte
	onPrefix   func()
	onTabClick func(col int)
	onTabNav   func(delta int)
	onWheel    func(delta int) // wheel the child doesn't consume (scrollback)
	onPage     func(delta int) // Ctrl+Shift+PageUp/Down: scroll a page of scrollback
	onAnyKey   func()          // non-mouse session input while scrolled back

	scrolled atomic.Bool // a scrollback view is active; keys snap it back

	prefix     byte
	carry      []byte       // partial escape-sequence candidate from the previous read
	mouseLevel atomic.Int32 // highest mouse mode the active session enabled (0 = none)

	stdin io.Reader
	done  chan struct{}
}

func NewRouter(prefix byte) *Router {
	if prefix == 0 {
		prefix = DefaultPrefix
	}
	return &Router{
		mode:   ModeChrome, // chrome until the first session exists
		toTea:  make(chan []byte, 32),
		stdin:  os.Stdin,
		done:   make(chan struct{}),
		prefix: prefix,
	}
}

// Prefix returns the configured prefix byte.
func (r *Router) Prefix() byte { return r.prefix }

// OnPrefix registers a callback invoked (from the stdin goroutine) when the
// prefix byte is seen in session mode. Use program.Send inside it.
func (r *Router) OnPrefix(f func()) { r.onPrefix = f }

// OnTabClick registers a callback for mouse presses on the tab bar row,
// reporting the 1-based column.
func (r *Router) OnTabClick(f func(col int)) { r.onTabClick = f }

// OnTabNav registers a callback for the Ctrl+Shift+Left/Right tab-switch
// shortcut (-1 / +1).
func (r *Router) OnTabNav(f func(delta int)) { r.onTabNav = f }

// OnWheel registers a callback for wheel events the child session has not
// asked to receive (mouse tracking off): tcc scrolls its own buffer.
// delta is negative for wheel-up (older content).
func (r *Router) OnWheel(f func(delta int)) { r.onWheel = f }

// OnPage registers a callback for the Ctrl+Shift+PageUp/PageDown shortcut,
// which scrolls tcc's scrollback a page at a time. delta is -1 for PageUp
// (older).
func (r *Router) OnPage(f func(delta int)) { r.onPage = f }

// OnAnyKey registers a callback fired when non-mouse input is forwarded to
// the session while a scrollback view is active (set via SetScrolled).
func (r *Router) OnAnyKey(f func()) { r.onAnyKey = f }

// SetScrolled tells the router whether the active tab is viewing scrollback;
// while true, forwarded keystrokes trigger OnAnyKey so the view snaps back.
func (r *Router) SetScrolled(on bool) { r.scrolled.Store(on) }

// SetMouseLevel sets the highest mouse-tracking mode the active session has
// enabled; session-area mouse reports beyond that level are dropped rather
// than typed into the child as garbage.
func (r *Router) SetMouseLevel(level int) { r.mouseLevel.Store(int32(level)) }

// SetActive sets the writer (PTY) that receives session-mode bytes.
func (r *Router) SetActive(w io.Writer) {
	r.mu.Lock()
	r.active = w
	r.mu.Unlock()
}

// SetMode switches routing between session and chrome mode.
func (r *Router) SetMode(m Mode) {
	r.mu.Lock()
	r.mode = m
	r.mu.Unlock()
}

func (r *Router) Mode() Mode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mode
}

// SendToActive writes bytes to the active PTY (e.g. the literal prefix byte
// for prefix-prefix).
func (r *Router) SendToActive(p []byte) {
	r.mu.Lock()
	w := r.active
	r.mu.Unlock()
	if w != nil {
		_, _ = w.Write(p)
	}
}

// Run reads stdin until the process ends. Call in a goroutine.
func (r *Router) Run() {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.stdin.Read(buf)
		if n > 0 {
			r.route(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			close(r.done)
			return
		}
	}
}

func (r *Router) route(p []byte) {
	if len(r.carry) > 0 {
		p = append(r.carry, p...)
		r.carry = nil
	}
	for len(p) > 0 {
		r.mu.Lock()
		mode := r.mode
		active := r.active
		r.mu.Unlock()

		if mode == ModeChrome {
			r.toTea <- p
			return
		}

		// Session mode: handle prefix and mouse rewriting.
		out, rest := r.filterSession(p)
		if len(out) > 0 && active != nil {
			_, _ = active.Write(out)
		}
		p = rest
	}
}

// filterSession scans session-mode bytes: forwards everything verbatim
// except the prefix byte (switches to chrome mode) and SGR mouse reports
// (row-shifted; tab-bar presses intercepted). Returns bytes to forward and
// the unprocessed remainder (after a prefix switch).
func (r *Router) filterSession(p []byte) (out, rest []byte) {
	out = make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		b := p[i]

		if b == r.prefix {
			r.SetMode(ModeChrome)
			if r.onPrefix != nil {
				r.onPrefix()
			}
			return out, p[i+1:]
		}

		if b == 0x1b {
			// Tab-switch shortcut: Ctrl+Shift+Left/Right (CSI 1;6 D/C).
			if delta, n, partial := scanTabNav(p[i:]); delta != 0 {
				if r.onTabNav != nil {
					r.onTabNav(delta)
				}
				i += n - 1
				continue
			} else if partial && n == len(p[i:]) && n >= 2 {
				r.carry = append([]byte(nil), p[i:]...)
				return out, nil
			}

			// Scroll shortcut: Ctrl+Shift+Up/Down (CSI 1;6 A/B) acts like
			// the mouse wheel on tcc's scrollback. (Partial reads share the
			// prefix with tab-nav and are carried above.)
			if delta, n := scanScrollNav(p[i:]); delta != 0 {
				if r.onWheel != nil {
					r.onWheel(delta)
				}
				i += n - 1
				continue
			}

			// Page-scroll shortcut: Ctrl+Shift+PageUp/Down (CSI 5;6~ / 6;6~)
			// scrolls tcc's scrollback a page at a time.
			if delta, n, partial := scanPageScroll(p[i:]); delta != 0 {
				if r.onPage != nil {
					r.onPage(delta)
				}
				i += n - 1
				continue
			} else if partial && n == len(p[i:]) && n >= 2 {
				r.carry = append([]byte(nil), p[i:]...)
				return out, nil
			}

			seq, n, complete := scanSGRMouse(p[i:])
			if !complete && n == len(p[i:]) && n >= 2 {
				// Mouse-sequence candidate cut off at the read boundary.
				// (A lone ESC is forwarded immediately — it's the ESC key,
				// and delaying it would break interrupt responsiveness.)
				r.carry = append([]byte(nil), p[i:]...)
				return out, nil
			}
			if complete {
				if fwd, click, col := rewriteMouseRow(seq); click {
					if r.onTabClick != nil {
						r.onTabClick(col)
					}
				} else if fwd != nil {
					if r.allowMouseEvent(seq) {
						out = append(out, fwd...)
					} else if delta, ok := wheelDelta(seq); ok && r.onWheel != nil {
						// The child doesn't want mouse events; wheel scrolls
						// tcc's own buffer for this tab.
						r.onWheel(delta)
					}
				}
				i += n - 1
				continue
			}
		}

		// Self-disarm on the first byte: multi-byte input (escape sequences,
		// pastes) must not flood the UI with one reset per byte. The app
		// re-arms via SetScrolled when the user scrolls again.
		if r.onAnyKey != nil && r.scrolled.CompareAndSwap(true, false) {
			r.onAnyKey()
		}
		out = append(out, b)
	}
	return out, nil
}

var (
	navNext    = []byte("\x1b[1;6C") // Ctrl+Shift+Right
	navPrev    = []byte("\x1b[1;6D") // Ctrl+Shift+Left
	scrollUp   = []byte("\x1b[1;6A") // Ctrl+Shift+Up
	scrollDown = []byte("\x1b[1;6B") // Ctrl+Shift+Down
	pageUp     = []byte("\x1b[5;6~") // Ctrl+Shift+PageUp
	pageDown   = []byte("\x1b[6;6~") // Ctrl+Shift+PageDown
)

// scanPageScroll matches the Ctrl+Shift+PageUp/PageDown shortcuts at the start of p.
// delta follows wheel semantics: -1 scrolls toward older content. partial=true
// means p ends mid-way through a possible match (n bytes examined). Unlike the
// scroll-nav sequences these share no prefix with tab-nav, so partials are
// carried here.
func scanPageScroll(p []byte) (delta, n int, partial bool) {
	for _, c := range [][]byte{pageUp, pageDown} {
		if bytes.HasPrefix(p, c) {
			if c[2] == '5' { // CSI 5;6~ = PageUp
				return -1, len(c), false
			}
			return 1, len(c), false
		}
		if len(p) < len(c) && bytes.HasPrefix(c, p) {
			return 0, len(p), true
		}
	}
	return 0, 0, false
}

// scanScrollNav matches the Ctrl+Shift+Up/Down scroll shortcuts at the start
// of p. delta follows wheel semantics: -1 scrolls toward older content.
// Partial matches are handled by scanTabNav's carry (shared prefix).
func scanScrollNav(p []byte) (delta, n int) {
	if bytes.HasPrefix(p, scrollUp) {
		return -1, len(scrollUp)
	}
	if bytes.HasPrefix(p, scrollDown) {
		return 1, len(scrollDown)
	}
	return 0, 0
}

// scanTabNav matches the Ctrl+Shift+Left/Right escape sequences at the start
// of p. delta is +1/-1 on a full match. partial=true means p ends mid-way
// through a possible match (n bytes examined).
func scanTabNav(p []byte) (delta, n int, partial bool) {
	for _, c := range [][]byte{navNext, navPrev} {
		if bytes.HasPrefix(p, c) {
			if c[len(c)-1] == 'C' {
				return 1, len(c), false
			}
			return -1, len(c), false
		}
		if len(p) < len(c) && bytes.HasPrefix(c, p) {
			return 0, len(p), true
		}
	}
	return 0, 0, false
}

// allowMouseEvent gates a session-bound SGR report by the mouse level the
// child actually enabled: hover motion needs 1003, drags need 1002+, and
// anything at all needs tracking on — otherwise the report would be typed
// into the child as garbage.
func (r *Router) allowMouseEvent(seq []byte) bool {
	level := int(r.mouseLevel.Load())
	if level == 0 {
		return false
	}
	// seq = ESC [ < b ; ... ; the leading param is the button/modifier code.
	b := 0
	for _, c := range seq[3:] {
		if c < '0' || c > '9' {
			break
		}
		b = b*10 + int(c-'0')
	}
	motion := b&32 != 0
	noButton := b&3 == 3 && b&64 == 0
	switch {
	case motion && noButton:
		return level >= 1003
	case motion:
		return level >= 1002
	default:
		return true
	}
}

// wheelDelta extracts the scroll direction from an SGR mouse report.
// Returns -1 for wheel-up (older content), +1 for wheel-down, ok=false for
// non-wheel events.
func wheelDelta(seq []byte) (int, bool) {
	b := 0
	for _, c := range seq[3:] {
		if c < '0' || c > '9' {
			break
		}
		b = b*10 + int(c-'0')
	}
	if b&64 == 0 {
		return 0, false
	}
	switch b & 3 {
	case 0:
		return -1, true // wheel up
	case 1:
		return 1, true // wheel down
	default:
		return 0, false // horizontal scroll (wheel left/right): ignore
	}
}

// scanSGRMouse checks whether p begins with an SGR mouse report
// (ESC [ < params M|m). It returns the candidate bytes, how many bytes were
// examined, and whether a full report was matched. n == len(p) with
// complete=false means the data ran out mid-candidate.
func scanSGRMouse(p []byte) (seq []byte, n int, complete bool) {
	const maxLen = 24
	prefix := []byte{0x1b, '[', '<'}
	for i := 0; i < len(p) && i < maxLen; i++ {
		if i < len(prefix) {
			if p[i] != prefix[i] {
				return nil, i, false
			}
			continue
		}
		c := p[i]
		switch {
		case c >= '0' && c <= '9' || c == ';':
			// param bytes
		case c == 'M' || c == 'm':
			return p[:i+1], i + 1, true
		default:
			return nil, i, false
		}
	}
	if len(p) < maxLen {
		return nil, len(p), false // ran out of data mid-candidate
	}
	return nil, maxLen, false
}

// rewriteMouseRow shifts an SGR mouse report up one row (the tab bar). For
// presses on the tab bar row it reports a click instead of forwarding.
// Returns the bytes to forward (nil to drop), whether this was a tab-bar
// press, and its column.
func rewriteMouseRow(seq []byte) (fwd []byte, tabClick bool, col int) {
	// seq = ESC [ < b ; x ; y (M|m)
	body := seq[3 : len(seq)-1]
	final := seq[len(seq)-1]
	var parts [3]int
	idx := 0
	for _, c := range body {
		if c == ';' {
			idx++
			if idx > 2 {
				return seq, false, 0 // malformed; forward untouched
			}
			continue
		}
		parts[idx] = parts[idx]*10 + int(c-'0')
	}
	if idx != 2 {
		return seq, false, 0
	}
	b, x, y := parts[0], parts[1], parts[2]
	if y <= 1 {
		// Tab bar row: report button presses (not motion/release/wheel).
		if final == 'M' && b&0x40 == 0 && b&0x20 == 0 {
			return nil, true, x
		}
		return nil, false, 0
	}
	out := make([]byte, 0, len(seq))
	out = append(out, 0x1b, '[', '<')
	out = appendInt(out, b)
	out = append(out, ';')
	out = appendInt(out, x)
	out = append(out, ';')
	out = appendInt(out, y-1)
	out = append(out, final)
	return out, false, 0
}

func appendInt(dst []byte, v int) []byte {
	if v == 0 {
		return append(dst, '0')
	}
	var tmp [8]byte
	i := len(tmp)
	for v > 0 && i > 0 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}
	return append(dst, tmp[i:]...)
}

// Read implements io.Reader for bubbletea's input.
func (r *Router) Read(p []byte) (int, error) {
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	select {
	case b := <-r.toTea:
		n := copy(p, b)
		r.pending = b[n:]
		return n, nil
	case <-r.done:
		return 0, io.EOF
	}
}
