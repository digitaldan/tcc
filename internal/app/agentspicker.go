package app

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitaldan/ctmux/internal/claude"
	"github.com/digitaldan/ctmux/internal/session"
)

// agentItem adapts a background agent to bubbles/list.
type agentItem struct{ a claude.Agent }

func (i agentItem) Title() string {
	name := i.a.Name
	if name == "" {
		name = i.a.Short
	}
	st, ok := claude.StateFromJob(i.a.State)
	glyph := "?"
	if ok {
		glyph = st.Glyph()
	}
	return fmt.Sprintf("%s %s", glyph, name)
}

func (i agentItem) Description() string {
	d := fmt.Sprintf("%s · %s", i.a.State, shortenHome(i.a.CWD))
	if i.a.WaitingFor != "" {
		d += " · waiting: " + i.a.WaitingFor
	}
	return d
}

func (i agentItem) FilterValue() string { return i.a.Name + " " + i.a.CWD + " " + i.a.Short }

type agentsPicker struct {
	list list.Model
}

func newAgentsPicker(width, height int) *agentsPicker {
	agents := claude.ListAgents()
	items := make([]list.Item, 0, len(agents))
	for _, a := range agents {
		items = append(items, agentItem{a})
	}

	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.Title = "background agents"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	return &agentsPicker{list: l}
}

func (m *Model) handleAgentsPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.agents.list.FilterState() != list.Filtering {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			m.enterSessionMode()
			return m, nil
		case "enter":
			item, ok := m.agents.list.SelectedItem().(agentItem)
			if !ok {
				m.enterSessionMode()
				return m, nil
			}
			m.attachAgent(item.a)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.agents.list, cmd = m.agents.list.Update(msg)
	return m, cmd
}

// attachAgent opens a tab running `claude attach <short>` whose status is
// driven by the daemon's job state file (hooks don't reach daemon workers).
func (m *Model) attachAgent(a claude.Agent) {
	title := a.Name
	if title == "" {
		title = a.Short
	}
	if err := m.spawnWith(session.SpawnOptions{
		Dir:    a.CWD,
		Attach: a.Short,
	}, tabTitle(title)); err != nil {
		return
	}

	t := m.activeTab()
	if st, ok := claude.StateFromJob(a.State); ok {
		t.status = st
		t.detail = a.WaitingFor
	}
	tabID := t.TabID
	if stop, err := claude.WatchJob(a.Short, func(js claude.JobState) {
		m.program.Send(jobStateMsg{tabID: tabID, js: js})
	}); err == nil {
		t.stopJobWatch = stop
	}
	m.enterSessionMode()
}

func (p *agentsPicker) view(width, rows int) string {
	p.list.SetSize(min(width-4, 100), rows-2)
	return lipgloss.NewStyle().Padding(1, 2).Render(p.list.View())
}

// applyJobState updates an attached tab from daemon job state.
func (t *tab) applyJobState(js claude.JobState) {
	if t.Exited() {
		return
	}
	if st, ok := claude.StateFromJob(js.State); ok {
		t.status = st
	}
	t.detail = js.WaitingFor
	if t.detail == "" {
		t.detail = js.Detail
	}
}
