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

// PrefixByte is the ctmux prefix key: Ctrl+Q (DC1, 0x11). Safe because raw
// mode disables IXON flow control and Claude Code does not bind Ctrl+Q.
const PrefixByte = 0x11

// Router owns stdin. In session mode bytes go straight to the active PTY so
// nothing is lost in re-encoding (paste blobs, modifiers, control chars). The
// prefix byte flips to chrome mode, where bytes flow to bubbletea instead.
// Router implements io.Reader for use with tea.WithInput.
type Router struct {
	mu     sync.Mutex
	mode   Mode
	active io.Writer

	toTea    chan []byte
	pending  []byte
	onPrefix func()

	stdin io.Reader
	done  chan struct{}
}

func NewRouter() *Router {
	return &Router{
		mode:  ModeChrome, // chrome until the first session exists
		toTea: make(chan []byte, 32),
		stdin: os.Stdin,
		done:  make(chan struct{}),
	}
}

// OnPrefix registers a callback invoked (from the stdin goroutine) when the
// prefix byte is seen in session mode. Use program.Send inside it.
func (r *Router) OnPrefix(f func()) { r.onPrefix = f }

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
// for Ctrl+Q Ctrl+Q).
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
	for len(p) > 0 {
		r.mu.Lock()
		mode := r.mode
		active := r.active
		r.mu.Unlock()

		if mode == ModeChrome {
			r.toTea <- p
			return
		}

		// Session mode: forward up to the prefix byte, if any.
		idx := indexByte(p, PrefixByte)
		if idx < 0 {
			if active != nil {
				_, _ = active.Write(p)
			}
			return
		}
		if idx > 0 && active != nil {
			_, _ = active.Write(p[:idx])
		}
		r.SetMode(ModeChrome)
		if r.onPrefix != nil {
			r.onPrefix()
		}
		p = p[idx+1:]
	}
}

func indexByte(p []byte, b byte) int {
	for i, c := range p {
		if c == b {
			return i
		}
	}
	return -1
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
