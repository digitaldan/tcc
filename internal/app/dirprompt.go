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

	"github.com/digitaldan/ctmux/internal/claude"
	"github.com/digitaldan/ctmux/internal/session"
)

var (
	promptBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("31")).
			Padding(0, 1)
	promptErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

	hereBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("31")).
			Padding(0, 2)
	herePathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	hereKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	hereDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	footerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// dirItem kinds in the browser list. Every row is a directory to navigate
// into — opening a session is a separate action (`o`) on the current
// directory shown in the header.
const (
	itemRecent = iota // a recent Claude project directory
	itemParent        // go up one level
	itemSubdir        // a subdirectory
)

type dirItem struct {
	kind int
	path string // absolute path the item navigates to
	name string // display name for subdirs
}

func (i dirItem) Title() string {
	switch i.kind {
	case itemRecent:
		return "★ " + filepath.Base(i.path)
	case itemParent:
		return "  ../"
	default:
		return "  " + i.name + "/"
	}
}

func (i dirItem) Description() string {
	switch i.kind {
	case itemRecent:
		return "  recent project · " + shortenHome(i.path)
	case itemParent:
		return "  up to " + shortenHome(i.path)
	default:
		return "  " + shortenHome(i.path)
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

// dirPrompt is the "new tab" overlay: a directory browser (navigation only)
// under a "start session here" header, with an optional manual path-entry
// mode.
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
	l.SetShowTitle(false)
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
	var items []list.Item

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
		case "o":
			// The only way to start a session: opens the current directory.
			return m.openSessionIn(d.curDir)
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
		case "enter", "right", "l":
			// Navigation only — never opens a session.
			if it, ok := d.list.SelectedItem().(dirItem); ok {
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

// view renders the "start session here" header, a divider, the navigation
// list, and a key-help footer.
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

	w := min(width-6, 100)

	// Action section: where the session will open.
	header := hereBoxStyle.Width(w).Render(
		hereKeyStyle.Render("o")+hereDimStyle.Render(" · start a new session in  ")+
			herePathStyle.Render(shortenHome(d.curDir)),
	) + "\n"

	// Divider between the action and the navigation sections.
	divider := hereDimStyle.Render(" ── browse ─ pick where to start "+strings.Repeat("─", max(0, w-33))) + "\n"

	d.list.SetSize(w, rows-8)
	body := d.list.View()
	if d.err != "" {
		body += "\n" + promptErrStyle.Render(d.err)
	}

	footer := footerStyle.Render("↑/↓: select · enter/→: enter dir · ←: up · ~: home · .: hidden dirs · /: filter · e: type a path · esc: cancel")

	return lipgloss.NewStyle().Padding(1, 2).Render(header + divider + body + "\n" + footer)
}
