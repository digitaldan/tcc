// Package app is the bubbletea TUI: tab bar, active-session view, and the
// prefix-chord key handling.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xterm "golang.org/x/term"

	"github.com/dcunningham/ctmux/internal/claude"
	"github.com/dcunningham/ctmux/internal/config"
	"github.com/dcunningham/ctmux/internal/session"
	"github.com/dcunningham/ctmux/internal/status"
	"github.com/dcunningham/ctmux/internal/term"
)

// tab couples a session with its UI state.
type tab struct {
	*session.Session
	status       status.State
	detail       string // e.g. notification message for needs_input
	stopJobWatch func() // attached tabs: stops the daemon job-state watcher
}

// uiMode is what the chrome is currently showing.
type uiMode int

const (
	uiSession      uiMode = iota // stdin routed to the active PTY
	uiChord                      // prefix pressed, awaiting chord key
	uiDirPrompt                  // "new tab" directory prompt
	uiResumePicker               // resume-a-session list
	uiAgentsPicker               // background agents list
)

type Model struct {
	program *tea.Program
	router  *term.Router

	sessions []*tab
	active   int

	width  int
	height int

	mode     uiMode
	quitting bool

	dirPrompt *dirPrompt
	resume    *resumePicker
	agents    *agentsPicker

	startDir     string
	settingsFile string
	stopWatcher  func()

	cfg       config.Config
	mouseOn   bool  // mouse tracking currently mirrored to the real terminal
	tabBounds []int // tab bar layout: end column (1-based) of each tab
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

	settingsFile, err := writeHooksSettings()
	if err != nil {
		return err
	}

	cfg := config.LoadConfig()
	m := &Model{
		router:       term.NewRouter(cfg.PrefixByte()),
		startDir:     dir,
		settingsFile: settingsFile,
		cfg:          cfg,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(m.router))
	m.program = p

	m.router.OnPrefix(func() {
		p.Send(prefixMsg{})
	})
	m.router.OnTabClick(func(col int) {
		p.Send(tabClickMsg{col: col})
	})
	go m.router.Run()

	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	cleanStaleState(stateDir)
	m.stopWatcher, err = status.Watch(stateDir, func(ev status.HookEvent) {
		p.Send(hookEventMsg{ev})
	})
	if err != nil {
		return err
	}

	_, err = p.Run()
	debugf("p.Run returned: %v", err)
	if m.stopWatcher != nil {
		m.stopWatcher()
	}
	for _, t := range m.sessions {
		t.Close()
	}
	debugf("app.Run exiting")
	return err
}

// cleanStaleState removes hook state files from previous ctmux runs.
func cleanStaleState(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// writeHooksSettings writes the --settings file pointing hooks at this
// binary's _hook subcommand.
func writeHooksSettings() (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	path, err := config.HooksSettingsPath()
	if err != nil {
		return "", err
	}
	if err := claude.WriteHooksSettings(path, bin); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Model) Init() tea.Cmd { return tickCmd() }

// tickCmd drives the busy-spinner animation and other periodic UI refresh.
func tickCmd() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

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

func (m *Model) tabByID(tabID string) *tab {
	for _, t := range m.sessions {
		if t.TabID == tabID {
			return t
		}
	}
	return nil
}

// spawn starts a new claude session in dir and makes it the active tab.
func (m *Model) spawn(dir string, extraArgs []string, kind session.Kind, title string) error {
	return m.spawnWith(session.SpawnOptions{
		Dir:          dir,
		SettingsFile: m.settingsFile,
		ExtraArgs:    extraArgs,
		PreassignID:  kind == session.KindSpawned,
	}, title)
}

// spawnWith starts a claude child from explicit options and makes it the
// active tab.
func (m *Model) spawnWith(opts session.SpawnOptions, title string) error {
	s := session.NewClaude(opts)
	if title != "" {
		s.Title = title
	}
	t := &tab{Session: s, status: status.Starting}
	tabID := s.TabID
	s.OnDamage = func() { m.program.Send(damageMsg{tabID: tabID}) }
	s.OnExit = func(code int) { m.program.Send(sessionExitMsg{tabID: tabID, code: code}) }
	s.OnBell = func() { m.program.Send(bellMsg{tabID: tabID}) }

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
	m.mode = uiSession
	m.dirPrompt = nil
	m.resume = nil
	m.agents = nil
	m.router.SetMode(term.ModeSession)
}

// closeTab kills the session and removes its tab. Quits when none remain.
func (m *Model) closeTab(i int) tea.Cmd {
	if i < 0 || i >= len(m.sessions) {
		return nil
	}
	if m.sessions[i].stopJobWatch != nil {
		m.sessions[i].stopJobWatch()
	}
	m.sessions[i].Close()
	m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
	if len(m.sessions) == 0 {
		m.quitting = true
		return tea.Quit
	}
	if m.active >= len(m.sessions) {
		m.active = len(m.sessions) - 1
	}
	m.setActive(m.active)
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case damageMsg, tickMsg:
	default:
		debugf("update: %T %v (mode=%v rmode=%v)", msg, msg, m.mode, m.router.Mode())
	}
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		first := m.width == 0
		m.width, m.height = msg.Width, msg.Height
		for _, t := range m.sessions {
			t.Resize(m.width, m.bodyRows())
		}
		if first && len(m.sessions) == 0 {
			if err := m.spawn(m.startDir, nil, session.KindSpawned, ""); err != nil {
				m.quitting = true
				return m, tea.Sequence(tea.Printf("ctmux: failed to start claude: %v", err), tea.Quit)
			}
			m.enterSessionMode()
		}
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		// Mirror the active session's mouse-tracking wish onto the real
		// terminal so wheel/clicks reach Claude (row-shifted by the router).
		want := false
		if t := m.activeTab(); t != nil && t.Term != nil && !t.Exited() {
			want = t.Term.MouseWanted()
		}
		if want != m.mouseOn {
			m.mouseOn = want
			if want {
				cmds = append(cmds, tea.EnableMouseAllMotion)
			} else {
				cmds = append(cmds, tea.DisableMouse)
			}
		}
		return m, tea.Batch(cmds...)

	case bellMsg:
		ringBell()
		return m, nil

	case tabClickMsg:
		for i, end := range m.tabBounds {
			if msg.col <= end {
				m.setActive(i)
				break
			}
		}
		return m, nil

	case prefixMsg:
		// Router already switched itself to chrome mode.
		m.mode = uiChord
		return m, nil

	case damageMsg:
		return m, nil // re-render

	case jobStateMsg:
		if t := m.tabByID(msg.tabID); t != nil {
			t.applyJobState(msg.js)
		}
		return m, nil

	case hookEventMsg:
		if t := m.tabByID(msg.ev.TabID); t != nil {
			if st, ok := status.FromHookEvent(msg.ev.Event); ok && !t.Exited() {
				t.status = st
				t.detail = msg.ev.Message
				// A background tab asking for input deserves a bell.
				if st == status.NeedsInput && m.activeTab() != t {
					ringBell()
				}
			}
			if msg.ev.SessionID != "" {
				t.SessionID = msg.ev.SessionID
			}
		}
		return m, nil

	case sessionExitMsg:
		if t := m.tabByID(msg.tabID); t != nil {
			t.ExitCode = msg.code
			if msg.code != 0 {
				t.status = status.Error
			} else {
				t.status = status.Exited
			}
		}
		if at := m.activeTab(); at != nil && at.TabID == msg.tabID {
			m.router.SetActive(nil)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case uiChord:
			return m.handleChord(msg)
		case uiDirPrompt:
			return m.handleDirPrompt(msg)
		case uiResumePicker:
			return m.handleResumePicker(msg)
		case uiAgentsPicker:
			return m.handleAgentsPicker(msg)
		default:
			// Bytes that raced into chrome mode while returning to session
			// mode: forward printable runes rather than dropping them.
			if m.router.Mode() == term.ModeSession && msg.Type == tea.KeyRunes {
				m.router.SendToActive([]byte(string(msg.Runes)))
				// Approving a permission prompt means Claude is working again.
				if t := m.activeTab(); t != nil && t.status == status.NeedsInput {
					t.status = status.Busy
				}
			}
			return m, nil
		}
	}
	// Anything else (FilterMatchesMsg, spinner ticks, cursor blinks, …) is a
	// component message for whichever overlay is open — bubbles/list filters
	// asynchronously, so dropping these breaks filtering entirely.
	return m.forwardToOverlay(msg)
}

// forwardToOverlay routes component messages to the active picker/prompt.
func (m *Model) forwardToOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.mode {
	case uiDirPrompt:
		if d := m.dirPrompt; d != nil {
			if d.manual {
				d.input, cmd = d.input.Update(msg)
			} else {
				d.list, cmd = d.list.Update(msg)
			}
		}
	case uiResumePicker:
		if m.resume != nil {
			m.resume.list, cmd = m.resume.list.Update(msg)
		}
	case uiAgentsPicker:
		if m.agents != nil {
			m.agents.list, cmd = m.agents.list.Update(msg)
		}
	}
	return m, cmd
}

// handleChord processes the key after the Ctrl+Q prefix.
func (m *Model) handleChord(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "d":
		m.quitting = true
		return m, tea.Quit
	case "c":
		m.mode = uiDirPrompt
		m.dirPrompt = newDirPrompt(m.startDir)
		return m, nil
	case "r":
		m.mode = uiResumePicker
		m.resume = newResumePicker(m.width, m.bodyRows())
		return m, nil
	case "a":
		m.mode = uiAgentsPicker
		m.agents = newAgentsPicker(m.width, m.bodyRows())
		return m, nil
	case "n", "tab":
		m.setActive((m.active + 1) % max(len(m.sessions), 1))
	case "p", "shift+tab":
		m.setActive((m.active - 1 + len(m.sessions)) % max(len(m.sessions), 1))
	case "x":
		if cmd := m.closeTab(m.active); cmd != nil {
			m.quitting = true
			return m, cmd
		}
	case "ctrl+" + string('a'+rune(m.router.Prefix())-1):
		// Prefix twice sends a literal prefix byte to the session.
		m.router.SendToActive([]byte{m.router.Prefix()})
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			m.setActive(int(key[0] - '1'))
		}
		// esc or anything else: cancel
	}
	m.enterSessionMode()
	return m, nil
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "starting…"
	}

	rows := m.bodyRows()
	var body string
	switch {
	case m.mode == uiDirPrompt && m.dirPrompt != nil:
		body = m.dirPrompt.view(m.width, rows)
	case m.mode == uiResumePicker && m.resume != nil:
		body = m.resume.view(m.width, rows)
	case m.mode == uiAgentsPicker && m.agents != nil:
		body = m.agents.view(m.width, rows)
	default:
		if t := m.activeTab(); t != nil && t.Term != nil {
			body = t.Term.View()
			if t.Exited() {
				body = overlayLine(body, m.width,
					fmt.Sprintf(" session exited (%d) — ^Q x to close tab, ^Q c for a new one ", t.ExitCode))
			}
		} else {
			body = "no session — press ^Q c to open one"
		}
	}

	// Body must be exactly bodyRows lines.
	lines := strings.Split(body, "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}

	return m.tabBar() + "\n" + strings.Join(lines, "\n")
}

// ringBell rings the real terminal's bell. A raw BEL byte is layout-safe.
func ringBell() {
	_, _ = os.Stdout.Write([]byte{7})
}

// overlayLine replaces the last line of body with a centered notice.
func overlayLine(body string, width int, notice string) string {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	style := chordStyle
	pad := (width - len(notice)) / 2
	if pad < 0 {
		pad = 0
	}
	lines[len(lines)-1] = strings.Repeat(" ", pad) + style.Render(notice)
	return strings.Join(lines, "\n")
}
