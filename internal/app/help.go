package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpView is the keybinding reference shown after the prefix + h/?.
func (m *Model) helpView(width, rows int) string {
	p := m.cfg.PrefixLabel()
	row := func(k, desc string) string {
		return "  " + splashKeyStyle.Render(p+" "+k) + "  " + splashDimStyle.Render(desc)
	}

	lines := []string{
		splashTitleStyle.Render("Keybindings"),
		"",
		splashDimStyle.Render("  Commands live behind the " + p + " prefix."),
		"",
		row("c", "new session — browse to a directory"),
		row("t", "new terminal — a plain shell in a directory"),
		row("r", "resume a session — pick from past Claude sessions"),
		row("a", "background agents — attach to a running agent"),
		row("n / p", "next / previous tab"),
		row("tab / ⇧tab", "next / previous tab"),
		row("1–9", "switch to tab by number"),
		row("x", "close the current tab"),
		row("d", "quit"),
		row("h / ?", "this help"),
		row(p, "send a literal "+p+" to the session"),
		"",
		splashDimStyle.Render("  In the new-session browser, w creates a git worktree."),
		splashDimStyle.Render("  ctrl+shift+←/→ switch tabs · ctrl+shift+↑/↓ scroll history · click a tab to focus"),
		"",
		splashDimStyle.Render("  press any key to close"),
	}

	box := splashBoxStyle.Render(strings.Join(lines, "\n"))

	pad := (rows - lipgloss.Height(box)) / 3
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat("\n", pad) + lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
}
