package term

import (
	"io"
	"os"
	"sync"
)

// Mode selects where raw stdin bytes are routed.
type Mode int32

const (
	// ModeSession forwards stdin bytes verbatim to the active session PTY.
	ModeSession Mode = iota
	// ModeChrome delivers stdin bytes to the TUI (bubbletea) for parsing.
	ModeChrome
)

// DefaultPrefix is the ctmux prefix key: Ctrl+Q (DC1, 0x11). Safe because raw
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

	prefix byte
	carry  []byte // partial mouse-sequence candidate from the previous read

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
					out = append(out, fwd...)
				}
				i += n - 1
				continue
			}
		}

		out = append(out, b)
	}
	return out, nil
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
