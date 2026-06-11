// Package app is the bubbletea TUI: tab bar, active-session view, and the
// prefix-chord key handling.
package app

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	xterm "golang.org/x/term"

	"github.com/dcunningham/ctmux/internal/session"
	"github.com/dcunningham/ctmux/internal/term"
)

// tab couples a session with its UI state.
type tab struct {
	*session.Session
}

// StatusGlyph is a placeholder until the hook-driven status engine (M1)
// lands: running vs exited only.
func (t *tab) StatusGlyph() string {
	if t.Exited() {
		if t.ExitCode != 0 {
			return "✕"
		}
		return "▢"
	}
	return "○"
}

type Model struct {
	program *tea.Program
	router  *term.Router

	sessions []*tab
	active   int

	width  int
	height int

	chordPending bool
	quitting     bool

	startDir string
}

func Run(args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	// Bubbletea only raw-modes its input when it owns the TTY; with a custom
	// input reader we must do it ourselves, or the line discipline eats our
	// prefix key (Ctrl+Q is XON) and buffers everything until newline.
	oldState, err := xterm.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("stdin is not a terminal: %w", err)
	}
	defer func() { _ = xterm.Restore(int(os.Stdin.Fd()), oldState) }()

	m := &Model{
		router:   term.NewRouter(),
		startDir: dir,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(m.router))
	m.program = p

	m.router.OnPrefix(func() {
		p.Send(prefixMsg{})
	})
	go m.router.Run()

	_, err = p.Run()
	debugf("p.Run returned: %v", err)
	for _, t := range m.sessions {
		t.Close()
	}
	debugf("app.Run exiting")
	return err
}

func (m *Model) Init() tea.Cmd { return nil }

// bodyRows returns the height available to sessions (terminal minus tab bar).
func (m *Model) bodyRows() int {
	r := m.height - 1
	if r < 1 {
		r = 1
	}
	return r
}

func (m *Model) activeTab() *tab {
	if m.active >= 0 && m.active < len(m.sessions) {
		return m.sessions[m.active]
	}
	return nil
}

// spawn starts a new claude session in dir and makes it the active tab.
func (m *Model) spawn(dir string) error {
	s := session.NewClaude(session.SpawnOptions{Dir: dir})
	t := &tab{Session: s}
	tabID := s.TabID
	s.OnDamage = func() { m.program.Send(damageMsg{tabID: tabID}) }
	s.OnExit = func(code int) { m.program.Send(sessionExitMsg{tabID: tabID, code: code}) }

	if err := s.Start(m.width, m.bodyRows()); err != nil {
		return err
	}
	m.sessions = append(m.sessions, t)
	m.setActive(len(m.sessions) - 1)
	return nil
}

// setActive switches the visible tab and points the input router at it.
func (m *Model) setActive(i int) {
	if i < 0 || i >= len(m.sessions) {
		return
	}
	m.active = i
	t := m.sessions[i]
	if t.Exited() {
		m.router.SetActive(nil)
	} else {
		m.router.SetActive(t.PTY)
	}
}

// enterSessionMode hands stdin back to the active session.
func (m *Model) enterSessionMode() {
	m.chordPending = false
	m.router.SetMode(term.ModeSession)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case damageMsg:
	default:
		debugf("update: %T %v (chord=%v mode=%v)", msg, msg, m.chordPending, m.router.Mode())
	}
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		first := m.width == 0
		m.width, m.height = msg.Width, msg.Height
		for _, t := range m.sessions {
			t.Resize(m.width, m.bodyRows())
		}
		if first && len(m.sessions) == 0 {
			if err := m.spawn(m.startDir); err != nil {
				m.quitting = true
				return m, tea.Sequence(tea.Printf("ctmux: failed to start claude: %v", err), tea.Quit)
			}
			m.enterSessionMode()
		}
		return m, nil

	case prefixMsg:
		// Router already switched itself to chrome mode.
		m.chordPending = true
		return m, nil

	case damageMsg:
		return m, nil // re-render

	case sessionExitMsg:
		for _, t := range m.sessions {
			if t.TabID == msg.tabID {
				t.ExitCode = msg.code
			}
		}
		if at := m.activeTab(); at != nil && at.TabID == msg.tabID {
			m.router.SetActive(nil)
		}
		// M0: single session — leaving means quitting.
		if len(m.sessions) == 1 {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyMsg:
		if m.chordPending {
			return m.handleChord(msg)
		}
		// Bytes that raced into chrome mode while returning to session mode:
		// forward printable runes to the active PTY rather than dropping them.
		if m.router.Mode() == term.ModeSession && msg.Type == tea.KeyRunes {
			m.router.SendToActive([]byte(string(msg.Runes)))
		}
		return m, nil
	}
	return m, nil
}

// handleChord processes the key after the Ctrl+Q prefix.
func (m *Model) handleChord(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	defer m.enterSessionMode()
	switch msg.String() {
	case "d", "q":
		m.quitting = true
		return m, tea.Quit
	case "ctrl+q":
		m.router.SendToActive([]byte{term.PrefixByte})
	}
	// esc or anything else: cancel
	return m, nil
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "starting…"
	}

	body := ""
	if t := m.activeTab(); t != nil && t.Term != nil {
		body = t.Term.View()
	} else {
		body = fmt.Sprintf("no session\n\npress ^Q d to quit")
	}

	// Body must be exactly bodyRows lines.
	lines := strings.Split(body, "\n")
	rows := m.bodyRows()
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}

	return m.tabBar() + "\n" + strings.Join(lines, "\n")
}
