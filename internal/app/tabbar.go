package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/digitaldan/tcc/internal/status"
)

var (
	tabBarStyle    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	tabActiveStyle = lipgloss.NewStyle().Background(lipgloss.Color("31")).Foreground(lipgloss.Color("231")).Bold(true)
	tabIdleStyle   = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("250"))
	chordStyle     = lipgloss.NewStyle().Background(lipgloss.Color("178")).Foreground(lipgloss.Color("16")).Bold(true)

	glyphColors = map[status.State]lipgloss.Color{
		status.Starting:   lipgloss.Color("244"),
		status.Busy:       lipgloss.Color("220"),
		status.Idle:       lipgloss.Color("114"),
		status.NeedsInput: lipgloss.Color("203"),
		status.Error:      lipgloss.Color("196"),
		status.Exited:     lipgloss.Color("240"),
	}
)

var spinnerFrames = []string{"✶", "✸", "✹", "✺"}

// glyphFor renders the status indicator, animating the busy spinner.
func glyphFor(st status.State) string {
	if st == status.Busy {
		frame := int(time.Now().UnixMilli()/400) % len(spinnerFrames)
		return spinnerFrames[frame]
	}
	return st.Glyph()
}

// tabBar renders the single-row tab bar across the top and records each
// tab's end column for click hit-testing.
func (m *Model) tabBar() string {
	var b strings.Builder
	used := 0
	m.tabBounds = m.tabBounds[:0]
	for i, t := range m.sessions {
		style := tabIdleStyle
		if i == m.active {
			style = tabActiveStyle
		}
		glyph := lipgloss.NewStyle().
			Background(style.GetBackground()).
			Foreground(glyphColors[t.status]).
			Render(glyphFor(t.status))
		label := fmt.Sprintf(" %d:%s ", i+1, t.Title)
		b.WriteString(style.Render(label) + glyph + style.Render(" "))
		used += lipgloss.Width(label) + 2
		m.tabBounds = append(m.tabBounds, used)
	}

	hint := ""
	if m.mode == uiSession {
		if t := m.activeTab(); t != nil && t.Term != nil {
			if off, total := t.Term.ScrollPosition(); off > 0 {
				hint = chordStyle.Render(fmt.Sprintf(" scroll %d/%d · wheel down or any key for live ", off, total))
			}
		}
	}
	switch m.mode {
	case uiChord:
		hint = chordStyle.Render(" " + m.cfg.PrefixLabel() + "  c:new r:resume a:agents n/p:switch x:close d:quit ")
	case uiDirPrompt:
		hint = chordStyle.Render(" new session ")
	case uiResumePicker:
		hint = chordStyle.Render(" resume session ")
	case uiAgentsPicker:
		hint = chordStyle.Render(" background agents ")
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
