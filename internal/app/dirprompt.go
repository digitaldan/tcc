package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dcunningham/ctmux/internal/claude"
	"github.com/dcunningham/ctmux/internal/session"
)

var (
	promptBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("31")).
			Padding(0, 1)
	promptErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// dirItem kinds in the browser list.
const (
	itemOpenHere = iota // open a session in the browsed directory
	itemRecent          // a recent Claude project directory; opens directly
	itemParent          // go up one level
	itemSubdir          // descend into a subdirectory
)

type dirItem struct {
	kind int
	path string // absolute path the item acts on
	name string // display name for subdirs
}

func (i dirItem) Title() string {
	switch i.kind {
	case itemOpenHere:
		return "▸ open session here"
	case itemRecent:
		return "★ " + shortenHome(i.path)
	case itemParent:
		return "▴ .."
	default:
		return "▸ " + i.name + "/"
	}
}

func (i dirItem) Description() string {
	switch i.kind {
	case itemOpenHere:
		return shortenHome(i.path)
	case itemRecent:
		return "recent project · enter opens"
	case itemParent:
		return shortenHome(i.path)
	default:
		return "enter to browse"
	}
}

func (i dirItem) FilterValue() string {
	switch i.kind {
	case itemSubdir:
		return i.name
	case itemRecent:
		return i.path
	default:
		return ""
	}
}

// dirPrompt is the "new tab" overlay: a directory browser with recents and
// an optional manual path-entry mode.
type dirPrompt struct {
	list    list.Model
	curDir  string
	recents []string
	atStart bool // recents are shown only in the initial view

	manual bool // manual path-entry mode
	input  textinput.Model
	err    string

	showHidden bool
}

func newDirPrompt(initial string) *dirPrompt {
	d := &dirPrompt{curDir: initial, atStart: true}
	d.recents = recentProjectDirs(initial, 6)

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "new session — pick a directory"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	d.list = l
	d.reload()

	ti := textinput.New()
	ti.Prompt = "dir: "
	d.input = ti
	return d
}

// recentProjectDirs returns unique, still-existing cwds from past Claude
// sessions, most recent first, excluding cur.
func recentProjectDirs(cur string, max int) []string {
	seen := map[string]bool{cur: true}
	var out []string
	for _, rs := range claude.ListSessions() {
		if seen[rs.Dir] {
			continue
		}
		seen[rs.Dir] = true
		if info, err := os.Stat(rs.Dir); err != nil || !info.IsDir() {
			continue
		}
		out = append(out, rs.Dir)
		if len(out) >= max {
			break
		}
	}
	return out
}

// reload rebuilds the list for curDir.
func (d *dirPrompt) reload() {
	items := []list.Item{dirItem{kind: itemOpenHere, path: d.curDir}}

	if d.atStart {
		for _, r := range d.recents {
			items = append(items, dirItem{kind: itemRecent, path: r})
		}
	}

	if parent := filepath.Dir(d.curDir); parent != d.curDir {
		items = append(items, dirItem{kind: itemParent, path: parent})
	}

	entries, err := os.ReadDir(d.curDir)
	if err != nil {
		d.err = fmt.Sprintf("cannot read %s: %v", d.curDir, err)
	} else {
		d.err = ""
		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if !d.showHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Slice(names, func(i, j int) bool {
			return strings.ToLower(names[i]) < strings.ToLower(names[j])
		})
		for _, n := range names {
			items = append(items, dirItem{kind: itemSubdir, path: filepath.Join(d.curDir, n), name: n})
		}
	}

	d.list.SetItems(items)
	d.list.ResetFilter()
	d.list.Select(0)
}

// navigate moves the browser to dir.
func (d *dirPrompt) navigate(dir string) {
	d.curDir = dir
	d.atStart = false
	d.reload()
}

func (m *Model) handleDirPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dirPrompt

	if d.manual {
		return m.handleManualEntry(msg)
	}

	if d.list.FilterState() != list.Filtering {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.enterSessionMode()
			return m, nil
		case "e":
			d.manual = true
			d.input.SetValue(d.curDir)
			d.input.CursorEnd()
			d.input.Focus()
			return m, nil
		case "~":
			if home, err := os.UserHomeDir(); err == nil {
				d.navigate(home)
			}
			return m, nil
		case ".":
			d.showHidden = !d.showHidden
			d.reload()
			return m, nil
		case "backspace", "left", "h":
			if parent := filepath.Dir(d.curDir); parent != d.curDir {
				d.navigate(parent)
			}
			return m, nil
		case "right", "l":
			if it, ok := d.list.SelectedItem().(dirItem); ok && it.kind == itemSubdir {
				d.navigate(it.path)
			}
			return m, nil
		case "enter":
			it, ok := d.list.SelectedItem().(dirItem)
			if !ok {
				return m, nil
			}
			switch it.kind {
			case itemOpenHere, itemRecent:
				return m.openSessionIn(it.path)
			case itemParent, itemSubdir:
				d.navigate(it.path)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	d.list, cmd = d.list.Update(msg)
	return m, cmd
}

func (m *Model) handleManualEntry(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dirPrompt
	switch msg.String() {
	case "esc":
		d.manual = false
		d.err = ""
		return m, nil
	case "ctrl+c":
		m.enterSessionMode()
		return m, nil
	case "enter":
		return m.openSessionIn(expandPath(d.input.Value()))
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return m, cmd
}

// openSessionIn validates dir and spawns a session there.
func (m *Model) openSessionIn(dir string) (tea.Model, tea.Cmd) {
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

// view renders the browser (or manual entry) panel.
func (d *dirPrompt) view(width, rows int) string {
	if d.manual {
		d.input.Width = max(20, min(width-20, 90))
		content := "new claude session — type a path\n\n" + d.input.View()
		if d.err != "" {
			content += "\n" + promptErrStyle.Render(d.err)
		}
		content += "\n\nenter: open · esc: back to browser"
		box := promptBoxStyle.Width(min(width-4, 100)).Render(content)
		pad := rows / 3
		return strings.Repeat("\n", pad) + lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
	}

	d.list.SetSize(min(width-6, 100), rows-4)
	body := d.list.View()
	if d.err != "" {
		body += "\n" + promptErrStyle.Render(d.err)
	}
	body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).
		Render("enter: open/browse · bksp: up · ~: home · .: hidden · /: filter · e: type path · esc: cancel")
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}
