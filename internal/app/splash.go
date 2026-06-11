package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	splashTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("31")).Bold(true)
	splashKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	splashDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	splashBoxStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("31")).
				Padding(1, 3)
)

const splashLogo = "      _\n" +
	"  ___| |_ _ __ ___  _   ___  __\n" +
	" / __| __| '_ ` _ \\| | | \\ \\/ /\n" +
	"| (__| |_| | | | | | |_| |>  <\n" +
	" \\___|\\__|_| |_| |_|\\__,_/_/\\_\\"

// splashView is shown when no sessions are open.
func (m *Model) splashView(width, rows int) string {
	key := func(k, desc string) string {
		return "  " + splashKeyStyle.Render(k) + "  " + desc
	}

	lines := []string{
		splashTitleStyle.Render(splashLogo),
		"",
		splashDimStyle.Render("  tabbed sessions for Claude Code"),
		"",
		key("c", "new session         — browse to a directory"),
		key("r", "resume a session    — pick from past Claude sessions"),
		key("a", "background agents   — attach to a running agent"),
		key("q", "quit"),
		"",
		splashDimStyle.Render("  inside a session, commands live behind the " + m.cfg.PrefixLabel() + " prefix:"),
		splashDimStyle.Render("  " + m.cfg.PrefixLabel() + " c new · r resume · a agents · n/p/1-9 switch · x close · d quit"),
		splashDimStyle.Render("  ctrl+shift+←/→ switch tabs · click a tab to focus it"),
	}

	box := splashBoxStyle.Render(strings.Join(lines, "\n"))

	pad := (rows - lipgloss.Height(box)) / 3
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat("\n", pad) + lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
}

// handleSplashKey handles bare keys on the splash screen (no prefix needed).
func (m *Model) handleSplashKey(key string) (handled bool, quit bool) {
	switch key {
	case "c", "enter":
		m.mode = uiDirPrompt
		m.dirPrompt = newDirPrompt(m.startDir)
		return true, false
	case "r":
		m.mode = uiResumePicker
		m.resume = newResumePicker(m.width, m.bodyRows())
		return true, false
	case "a":
		m.mode = uiAgentsPicker
		m.agents = newAgentsPicker(m.width, m.bodyRows())
		return true, false
	case "q", "d", "ctrl+c":
		return true, true
	}
	return false, false
}
