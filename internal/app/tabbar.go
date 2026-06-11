package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabBarStyle    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	tabActiveStyle = lipgloss.NewStyle().Background(lipgloss.Color("31")).Foreground(lipgloss.Color("231")).Bold(true)
	tabIdleStyle   = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	chordStyle     = lipgloss.NewStyle().Background(lipgloss.Color("178")).Foreground(lipgloss.Color("16")).Bold(true)
)

// tabBar renders the single-row tab bar across the top.
func (m *Model) tabBar() string {
	var b strings.Builder
	used := 0
	for i, s := range m.sessions {
		label := fmt.Sprintf(" %d:%s %s ", i+1, s.Title, s.StatusGlyph())
		style := tabIdleStyle
		if i == m.active {
			style = tabActiveStyle
		}
		b.WriteString(style.Render(label))
		used += lipgloss.Width(label)
	}

	hint := ""
	if m.chordPending {
		hint = chordStyle.Render(" ^Q… d:quit  esc:cancel ")
	}
	hintW := lipgloss.Width(hint)

	pad := m.width - used - hintW
	if pad < 0 {
		pad = 0
	}
	b.WriteString(tabBarStyle.Render(strings.Repeat(" ", pad)))
	b.WriteString(hint)
	return b.String()
}
