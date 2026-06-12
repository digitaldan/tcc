package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/digitaldan/tcc/internal/claude"
	"github.com/digitaldan/tcc/internal/config"
	"github.com/digitaldan/tcc/internal/session"
)

// savedTab is one tab's identity, enough to reopen it after a quit or crash.
type savedTab struct {
	Kind       string `json:"kind"` // "spawned" | "resumed" | "attached"
	SessionID  string `json:"session_id,omitempty"`
	AgentShort string `json:"agent_short,omitempty"`
	Dir        string `json:"dir,omitempty"`
	Title      string `json:"title,omitempty"`
}

// savedState is the persisted tab set, written to ~/.tcc/tabs.json on every
// tab change so a crash loses nothing.
type savedState struct {
	Tabs   []savedTab `json:"tabs"`
	Active int        `json:"active"`
}

func tabsFilePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tabs.json"), nil
}

func kindString(k session.Kind) string {
	switch k {
	case session.KindResumed:
		return "resumed"
	case session.KindAttached:
		return "attached"
	default:
		return "spawned"
	}
}

// saveTabs snapshots the current tab set. Errors are ignored — persistence
// is best-effort and must never disturb the UI.
func (m *Model) saveTabs() {
	if m.restoring {
		return // restoreTabs saves once when done
	}
	path, err := tabsFilePath()
	if err != nil {
		return
	}
	st := savedState{Active: m.active, Tabs: make([]savedTab, 0, len(m.sessions))}
	for _, t := range m.sessions {
		st.Tabs = append(st.Tabs, savedTab{
			Kind:       kindString(t.Kind),
			SessionID:  t.SessionID,
			AgentShort: t.AgentShort,
			Dir:        t.Dir,
			Title:      t.Title,
		})
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tabs-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
	}
}

func loadSavedTabs() savedState {
	var st savedState
	path, err := tabsFilePath()
	if err != nil {
		return st
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

// restoreTabs reopens the previously saved tab set: attached agents
// re-attach when their worker is still alive (resuming otherwise), and
// everything else resumes its session with history. Tabs whose session
// vanished are skipped.
func (m *Model) restoreTabs(st savedState) {
	debugf("restoreTabs: %d saved tabs, size %dx%d", len(st.Tabs), m.width, m.height)
	m.restoring = true // suppress per-tab saveTabs churn; saved once below
	for _, s := range st.Tabs {
		debugf("restore tab kind=%s sid=%s dir=%s resumable=%v", s.Kind, s.SessionID, s.Dir, claude.SessionResumable(s.SessionID))
		if s.Kind == "attached" {
			if a, ok := claude.LiveAgentByShort(s.AgentShort); ok {
				m.attachAgent(a)
				continue
			}
			// Worker gone; fall through to resume the conversation instead.
		}
		dir := s.Dir
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = m.startDir
		}
		if claude.SessionResumable(s.SessionID) {
			if err := m.spawn(dir, []string{"--resume", s.SessionID}, session.KindResumed, s.Title); err != nil {
				debugf("restoreTabs: resume %s failed: %v", s.SessionID, err)
			}
		} else {
			// Nothing to resume (never prompted, conversation never hit
			// disk, or transcript deleted); keep the tab by reopening a
			// fresh session in its directory.
			if err := m.spawn(dir, nil, session.KindSpawned, s.Title); err != nil {
				debugf("restoreTabs: fresh spawn in %s failed: %v", dir, err)
			}
		}
	}
	m.restoring = false
	if len(m.sessions) > 0 {
		if st.Active >= 0 && st.Active < len(m.sessions) {
			m.setActive(st.Active)
		}
		m.enterSessionMode()
	}
	m.saveTabs() // reflect what actually reopened
}
