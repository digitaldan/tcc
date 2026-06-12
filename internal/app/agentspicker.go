package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/digitaldan/tcc/internal/claude"
	"github.com/digitaldan/tcc/internal/session"
	"github.com/digitaldan/tcc/internal/status"
)

// agentItem adapts a background agent to bubbles/list. openTab is the
// 1-based tab number when the agent is already open in tcc (0 = not).
type agentItem struct {
	a       claude.Agent
	openTab int
}

// agentState maps the agent onto tcc's status model for glyphs/colors.
func (i agentItem) agentState() status.State {
	if st, ok := claude.StateFromJob(i.a.State); ok {
		return st
	}
	if i.a.Live {
		return status.Busy // live worker with unknown state (adopted /background session)
	}
	return status.Exited
}

func (i agentItem) Title() string {
	st := i.agentState()
	name := i.a.Name
	if name == "" {
		name = i.a.Short
	}
	glyph := lipgloss.NewStyle().Foreground(glyphColors[st]).Render(st.Glyph())
	return glyph + " " + name
}

func (i agentItem) Description() string {
	var parts []string

	if i.openTab > 0 {
		parts = append(parts, fmt.Sprintf("open in tab %d — enter switches", i.openTab))
	}

	state := i.a.State
	if state == "" {
		if i.a.Live {
			state = "running"
		} else {
			state = "stopped"
		}
	}
	parts = append(parts, state)

	if i.a.WaitingFor != "" {
		parts = append(parts, "waiting: "+i.a.WaitingFor)
	}
	if d := strings.TrimSpace(i.a.Detail); d != "" {
		parts = append(parts, truncateTo(d, 64))
	}
	if !i.a.UpdatedAt.IsZero() {
		parts = append(parts, humanAge(time.Since(i.a.UpdatedAt)))
	}
	return strings.Join(parts, " · ")
}

func (i agentItem) FilterValue() string { return i.a.Name + " " + i.a.CWD + " " + i.a.Short }

func truncateTo(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

type agentsPicker struct {
	list     list.Model
	stopping bool       // a worker is being stopped before resuming with history
	confirm  *agentItem // pending x-action awaiting y/N
	busy     string     // progress notice while a destructive action runs
	notice   string     // transient hint (e.g. "close its tab first")
}

func newAgentsPicker(m *Model, width, height int) *agentsPicker {
	agents := claude.ListAgents()
	items := make([]list.Item, 0, len(agents))
	for _, a := range agents {
		item := agentItem{a: a}
		if i := m.tabIndexBySessionID(a.SessionID); i >= 0 {
			item.openTab = i + 1
		} else if i := m.tabIndexByAgentShort(a.Short); i >= 0 {
			item.openTab = i + 1
		}
		items = append(items, item)
	}

	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(lipgloss.Color("231"))
	l := list.New(items, d, width, height)
	l.Title = "background agents"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	return &agentsPicker{list: l}
}

func (m *Model) handleAgentsPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.agents
	if p.busy != "" {
		return m, nil // action in flight; ignore keys
	}
	if p.confirm != nil {
		item := *p.confirm
		p.confirm = nil
		if s := msg.String(); s == "y" || s == "Y" {
			if item.a.Live {
				p.busy = "stopping background agent…"
			} else {
				p.busy = "removing from agents list…"
			}
			return m, agentRemoveCmd(item.a)
		}
		return m, nil // anything else cancels
	}
	p.notice = ""

	if m.agents.list.FilterState() != list.Filtering {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			m.enterSessionMode()
			return m, nil
		case "x":
			item, ok := m.agents.list.SelectedItem().(agentItem)
			if !ok {
				return m, nil
			}
			if item.openTab > 0 {
				p.notice = fmt.Sprintf("agent is open in tab %d — close the tab first", item.openTab)
				return m, nil
			}
			p.confirm = &item
			return m, nil
		case "enter":
			item, ok := m.agents.list.SelectedItem().(agentItem)
			if !ok {
				m.enterSessionMode()
				return m, nil
			}
			if m.switchToOpen(item.a.SessionID, item.a.Short) {
				return m, nil
			}
			if item.a.Live {
				m.attachAgent(item.a)
				return m, nil
			}
			// No worker running: open the conversation with full history.
			return m, m.resumeAgent(item.a)
		case "s":
			item, ok := m.agents.list.SelectedItem().(agentItem)
			if !ok || item.a.SessionID == "" {
				return m, nil
			}
			if m.switchToOpen(item.a.SessionID, item.a.Short) {
				return m, nil
			}
			return m, m.resumeAgent(item.a)
		}
	}
	var cmd tea.Cmd
	m.agents.list, cmd = m.agents.list.Update(msg)
	return m, cmd
}

// resumeAgent opens the agent's conversation interactively with history,
// stopping its worker first when one is running.
func (m *Model) resumeAgent(a claude.Agent) tea.Cmd {
	title := a.Name
	if title == "" {
		title = a.Short
	}
	if !a.Live {
		// Nothing to stop; resume directly.
		return func() tea.Msg {
			return agentStoppedMsg{sessionID: a.SessionID, dir: a.CWD, title: title}
		}
	}
	m.agents.stopping = true
	return stopAndResume(a, a.CWD, title)
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
	t.SessionID = a.SessionID
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
	if p.stopping {
		return lipgloss.NewStyle().Padding(2, 4).
			Render("stopping background agent, then resuming with history…")
	}
	if p.busy != "" {
		return lipgloss.NewStyle().Padding(2, 4).Render(p.busy)
	}
	p.list.SetSize(min(width-4, 110), rows-3)
	body := p.list.View()
	switch {
	case p.confirm != nil:
		name := p.confirm.a.Name
		if name == "" {
			name = p.confirm.a.Short
		}
		what := "Remove \"" + tabTitle(name) + "\" from this list? Its conversation stays resumable."
		if p.confirm.a.Live {
			what = "Stop background agent \"" + tabTitle(name) + "\"? Its conversation stays resumable."
		}
		body += "\n" + confirmStyle.Render(" "+what+"  y: yes · any other key: cancel ")
	case p.notice != "":
		body += "\n" + noticeStyle.Render(p.notice)
	default:
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
			"enter: open (live attach · finished resume) · s: stop & resume live · x: stop / remove · /: filter · esc: cancel")
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

// agentRemoveCmd stops a live worker, or removes a finished agent's job
// record from the list. The conversation transcript is untouched.
func agentRemoveCmd(a claude.Agent) tea.Cmd {
	return func() tea.Msg {
		if a.Live {
			_ = claude.StopAgent(a.Short)
			claude.WaitAgentGone(a.SessionID, 5*time.Second)
		} else {
			_ = claude.RemoveJob(a.Short)
		}
		return pickerRefreshMsg{mode: uiAgentsPicker}
	}
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
