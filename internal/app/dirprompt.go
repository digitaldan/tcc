package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dcunningham/ctmux/internal/session"
)

var (
	promptBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("31")).
			Padding(0, 1)
	promptErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// dirPrompt is the "new tab" overlay: type a directory, Enter to spawn.
type dirPrompt struct {
	input textinput.Model
	err   string
}

func newDirPrompt(initial string) *dirPrompt {
	ti := textinput.New()
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Focus()
	ti.Prompt = "dir: "
	return &dirPrompt{input: ti}
}

// expandPath resolves ~ and makes the path absolute.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func (m *Model) handleDirPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.enterSessionMode()
		return m, nil
	case "enter":
		dir := expandPath(m.dirPrompt.input.Value())
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			m.dirPrompt.err = fmt.Sprintf("not a directory: %s", dir)
			return m, nil
		}
		if err := m.spawn(dir, nil, session.KindSpawned, ""); err != nil {
			m.dirPrompt.err = fmt.Sprintf("spawn failed: %v", err)
			return m, nil
		}
		m.enterSessionMode()
		return m, nil
	}
	var cmd tea.Cmd
	m.dirPrompt.input, cmd = m.dirPrompt.input.Update(msg)
	return m, cmd
}

// view renders the prompt panel over an empty body.
func (d *dirPrompt) view(width, rows int) string {
	d.input.Width = max(20, width-20)

	content := "new claude session\n\n" + d.input.View()
	if d.err != "" {
		content += "\n" + promptErrStyle.Render(d.err)
	}
	content += "\n\nenter: open · esc: cancel"

	box := promptBoxStyle.Width(min(width-4, 80)).Render(content)

	// Vertically offset a third of the way down.
	pad := rows / 3
	return strings.Repeat("\n", pad) + lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
}
