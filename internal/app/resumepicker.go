package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitaldan/tcc/internal/claude"
	"github.com/digitaldan/tcc/internal/session"
)

// resumeItem adapts a ResumableSession to bubbles/list. live is non-nil when
// the session is currently running as a background worker — resuming then
// requires stopping the worker first (claude refuses otherwise). openTab is
// the 1-based tab number when the session is already open in tcc (0 = not).
type resumeItem struct {
	rs      claude.ResumableSession
	live    *claude.Agent
	openTab int
}

func (i resumeItem) Title() string {
	t := i.rs.Title
	switch {
	case i.openTab > 0:
		t = "▸ " + t
	case i.live != nil:
		t = "● " + t
	case i.rs.Background:
		t += "  (bg)"
	}
	return t
}

func (i resumeItem) Description() string {
	if i.openTab > 0 {
		return fmt.Sprintf("already open in tab %d — enter switches to it", i.openTab)
	}
	if i.live != nil {
		return "running as background agent — enter stops it and resumes here"
	}
	dir := shortenHome(i.rs.Dir)
	branch := ""
	if i.rs.GitBranch != "" {
		branch = " (" + i.rs.GitBranch + ")"
	}
	return fmt.Sprintf("%s%s · %s", dir, branch, humanAge(time.Since(i.rs.Modified)))
}

func (i resumeItem) FilterValue() string { return i.rs.Title + " " + i.rs.Dir }

type resumePicker struct {
	list     list.Model
	stopping bool // a background worker is being stopped before resume
}

func newResumePicker(m *Model, width, height int) *resumePicker {
	sessions := claude.ListSessions()
	active := claude.ActiveAgentsBySession()
	items := make([]list.Item, 0, len(sessions))
	for _, rs := range sessions {
		item := resumeItem{rs: rs}
		if i := m.tabIndexBySessionID(rs.SessionID); i >= 0 {
			item.openTab = i + 1
		}
		if a, ok := active[rs.SessionID]; ok {
			a := a
			item.live = &a
		}
		items = append(items, item)
	}

	d := list.NewDefaultDelegate()
	l := list.New(items, d, width, height)
	l.Title = "resume a session"
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.DisableQuitKeybindings()
	return &resumePicker{list: l}
}

func (m *Model) handleResumePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Let the list's filter input see keys first when filtering.
	if m.resume.list.FilterState() != list.Filtering {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			m.enterSessionMode()
			return m, nil
		case "enter":
			item, ok := m.resume.list.SelectedItem().(resumeItem)
			if !ok {
				m.enterSessionMode()
				return m, nil
			}
			rs := item.rs
			if m.switchToOpen(rs.SessionID, "") {
				return m, nil
			}
			if item.live != nil {
				// Worker must be stopped before the session can be resumed.
				m.resume.stopping = true
				return m, stopAndResume(*item.live, rs.Dir, rs.Title)
			}
			dir := rs.Dir
			if _, err := os.Stat(dir); err != nil {
				dir, _ = os.Getwd() // directory vanished; resume from cwd
			}
			if err := m.spawn(dir, []string{"--resume", rs.SessionID}, session.KindResumed, tabTitle(rs.Title)); err == nil {
				m.enterSessionMode()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.resume.list, cmd = m.resume.list.Update(msg)
	return m, cmd
}

func (p *resumePicker) view(width, rows int) string {
	if p.stopping {
		return lipgloss.NewStyle().Padding(2, 4).
			Render("stopping background agent, then resuming with history…")
	}
	p.list.SetSize(min(width-4, 100), rows-2)
	return lipgloss.NewStyle().Padding(1, 2).Render(p.list.View())
}

// tabTitle shortens a session title to fit the tab bar.
func tabTitle(s string) string {
	r := []rune(s)
	if len(r) > 22 {
		return string(r[:21]) + "…"
	}
	return s
}

func shortenHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
