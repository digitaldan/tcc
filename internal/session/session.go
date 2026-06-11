// Package session manages one Claude Code child process: its PTY, embedded
// terminal emulator, and lifecycle.
package session

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/dcunningham/ctmux/internal/term"
)

// Kind describes how the session's claude process was started.
type Kind int

const (
	KindSpawned Kind = iota // fresh `claude` in a directory
	KindResumed             // `claude --resume <id>`
	KindAttached            // `claude attach <short-id>` (background agent)
)

type Session struct {
	TabID     string // ctmux tab UUID; keys hook state files
	SessionID string // claude session UUID (pre-assigned or learned from hooks)
	Title     string
	Dir       string
	Kind      Kind

	Cmd  *exec.Cmd
	PTY  *os.File
	Term *term.Emulator

	// OnDamage is called (coalesced, from a goroutine) when output changed.
	OnDamage func()
	// OnExit is called once from a goroutine when the child exits.
	OnExit func(exitCode int)

	dirty    atomic.Bool
	exited   atomic.Bool
	ExitCode int

	closeOnce sync.Once
}

// damageInterval coalesces redraw notifications per session.
const damageInterval = 33 * time.Millisecond

// Start launches the command in a new PTY sized cols x rows and wires the
// emulator: PTY output feeds the screen, emulator responses (answers to the
// child's terminal queries) flow back into the PTY.
func (s *Session) Start(cols, rows int) error {
	s.Term = term.New(cols, rows)

	f, err := pty.StartWithSize(s.Cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return err
	}
	s.PTY = f

	notify := make(chan struct{}, 1)

	// emulator responses -> PTY (capability query answers). Must start before
	// the PTY reader: Feed blocks on the response pipe until it's drained.
	go func() {
		_, _ = io.Copy(f, s.Term.Responses())
	}()

	// PTY -> emulator
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.Term.Feed(buf[:n])
				if !s.dirty.Swap(true) {
					select {
					case notify <- struct{}{}:
					default:
					}
				}
			}
			if err != nil {
				return // PTY closed; exit watcher handles the rest
			}
		}
	}()

	// Coalesced damage notifications
	go func() {
		for range notify {
			s.dirty.Store(false)
			if s.OnDamage != nil {
				s.OnDamage()
			}
			time.Sleep(damageInterval)
			if s.dirty.Load() {
				select {
				case notify <- struct{}{}:
				default:
				}
			}
		}
	}()

	// exit watcher
	go func() {
		err := s.Cmd.Wait()
		code := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if err != nil {
			code = -1
		}
		s.ExitCode = code
		s.exited.Store(true)
		close(notify)
		if s.OnExit != nil {
			s.OnExit(code)
		}
	}()

	return nil
}

func (s *Session) Exited() bool { return s.exited.Load() }

func (s *Session) Resize(cols, rows int) {
	if s.PTY != nil && !s.exited.Load() {
		_ = pty.Setsize(s.PTY, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}
	if s.Term != nil {
		s.Term.Resize(cols, rows)
	}
}

// Close terminates the child (SIGTERM, then SIGKILL after a grace period)
// and releases the PTY and emulator.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		if s.Cmd != nil && s.Cmd.Process != nil && !s.exited.Load() {
			_ = s.Cmd.Process.Signal(syscall.SIGTERM)
			go func() {
				time.Sleep(3 * time.Second)
				if !s.exited.Load() {
					_ = s.Cmd.Process.Kill()
				}
			}()
		}
		if s.PTY != nil {
			_ = s.PTY.Close()
		}
		if s.Term != nil {
			s.Term.Close()
		}
	})
}
