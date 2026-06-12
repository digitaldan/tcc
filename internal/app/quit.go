package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitaldan/tcc/internal/session"
	"github.com/digitaldan/tcc/internal/status"
)

// busyTabs returns tabs that are mid-task: their Claude is working and would
// lose in-flight work on quit. Attached agents are excluded — their workers
// live in the daemon and survive quit by design.
func (m *Model) busyTabs() []*tab {
	var out []*tab
	for _, t := range m.sessions {
		if t.Kind != session.KindAttached && !t.Exited() && t.status == status.Busy {
			out = append(out, t)
		}
	}
	return out
}

// requestQuit quits immediately when nothing is mid-task; otherwise it warns
// and asks for confirmation first.
func (m *Model) requestQuit() (tea.Model, tea.Cmd) {
	busy := m.busyTabs()
	debugf("requestQuit: %d busy of %d tabs", len(busy), len(m.sessions))
	if len(busy) == 0 {
		return m.quitNow()
	}
	m.mode = uiQuitConfirm
	return m, nil
}

func (m *Model) quitNow() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Quit
}

// handleQuitConfirm processes the quit warning: Enter/y quits anyway,
// anything else cancels.
func (m *Model) handleQuitConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y", "Y", "d":
		return m.quitNow()
	default: // esc, n, or anything else cancels the quit
		m.enterSessionMode()
		return m, nil
	}
}

// quitConfirmView renders the not-idle quit warning.
func (m *Model) quitConfirmView(width, rows int) string {
	busy := m.busyTabs()
	names := make([]string, 0, len(busy))
	for _, t := range busy {
		names = append(names, "  ✶ "+t.Title)
	}

	content := fmt.Sprintf("%d session(s) are still working:\n\n", len(busy)) +
		strings.Join(names, "\n") + "\n\n" +
		hereDimStyle.Render("quitting stops them — their conversations stay resumable\n"+
			"and these tabs reopen on the next launch") + "\n\n" +
		hereKeyStyle.Render("enter/y") + hereDimStyle.Render("  quit anyway") + "\n" +
		hereKeyStyle.Render("esc") + hereDimStyle.Render("      cancel")

	box := splashBoxStyle.Render(content)
	pad := (rows - lipgloss.Height(box)) / 3
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat("\n", pad) + lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
}
