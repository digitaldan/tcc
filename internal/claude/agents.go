package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/digitaldan/ctmux/internal/config"
	"github.com/digitaldan/ctmux/internal/status"
)

// Agent is a Claude Code background session managed by the daemon.
type Agent struct {
	Short      string `json:"id"`
	PID        int    `json:"pid"`
	CWD        string `json:"cwd"`
	Kind       string `json:"kind"` // "interactive" | "background"
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	Status     string `json:"status"` // "idle" | "waiting" | ...
	State      string `json:"state"`  // "working" | "blocked" | "done" | "failed" | "stopped"
	WaitingFor string `json:"waitingFor"`

	// Enriched from the job state file; not part of the CLI JSON.
	Detail    string    `json:"-"` // last progress/result summary
	UpdatedAt time.Time `json:"-"`
	Live      bool      `json:"-"` // a worker process is currently running
}

// ListAgents returns background sessions like the native agent view: live
// workers (from `claude agents --json`, falling back to the roster) unioned
// with completed/stopped jobs from ~/.claude/jobs, newest first with
// attention-needing agents on top.
func ListAgents() []Agent {
	byShort := map[string]*Agent{}
	var order []string

	live, err := listAgentsCLI()
	if err != nil {
		live = listAgentsFromFiles()
	}
	for _, a := range live {
		if a.Kind != "background" || a.Short == "" {
			continue
		}
		a := a
		a.Live = true
		byShort[a.Short] = &a
		order = append(order, a.Short)
	}

	// Union with job state files: enrich live agents, add completed ones.
	jobs, _ := os.ReadDir(jobsDir())
	for _, d := range jobs {
		if !d.IsDir() {
			continue
		}
		short := d.Name()
		js, err := readJobState(short)
		if err != nil {
			continue
		}
		if a, ok := byShort[short]; ok {
			a.Detail = js.Detail
			a.UpdatedAt = js.UpdatedTime()
			if a.Name == "" || a.Name == a.Short {
				a.Name = js.DisplayName()
			}
			if a.State == "" {
				a.State = js.State
			}
			continue
		}
		byShort[short] = &Agent{
			Short:      short,
			CWD:        js.CWD,
			Kind:       "background",
			SessionID:  js.SessionID,
			Name:       js.DisplayName(),
			State:      js.State,
			WaitingFor: js.WaitingFor,
			Detail:     js.Detail,
			UpdatedAt:  js.UpdatedTime(),
		}
		order = append(order, short)
	}

	out := make([]Agent, 0, len(order))
	for _, short := range order {
		a := *byShort[short]
		// Adopted /background sessions have no job name; prefer the session's
		// transcript title, then the project directory, over a bare short id.
		if a.Name == "" || a.Name == a.Short {
			if title := TranscriptTitle(a.SessionID); title != "" {
				a.Name = title
			} else if a.CWD != "" {
				a.Name = filepath.Base(a.CWD) + " (backgrounded)"
			}
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := statePriority(out[i]), statePriority(out[j])
		if pi != pj {
			return pi < pj
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// statePriority orders agents like the native view: needs-input, working,
// then finished.
func statePriority(a Agent) int {
	switch a.State {
	case "blocked":
		return 0
	case "working":
		return 1
	default:
		if a.Live {
			return 1 // live worker with unknown state (e.g. adopted /background session)
		}
		return 2
	}
}

func listAgentsCLI() ([]Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "claude", "agents", "--json").Output()
	if err != nil {
		return nil, err
	}
	var agents []Agent
	if err := json.Unmarshal(out, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// JobState is the daemon's per-job state.json, the status source for
// attached tabs (our hooks don't reach daemon-spawned workers).
type JobState struct {
	State      string `json:"state"`
	Detail     string `json:"detail"`
	WaitingFor string `json:"waitingFor"`
	Name       string `json:"name"`
	Intent     string `json:"intent"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	UpdatedAt  string `json:"updatedAt"`
}

// DisplayName prefers the job's name, falling back to its intent.
func (js JobState) DisplayName() string {
	if js.Name != "" {
		return js.Name
	}
	return js.Intent
}

// UpdatedTime parses the job's updatedAt timestamp.
func (js JobState) UpdatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, js.UpdatedAt)
	return t
}

func jobsDir() string { return filepath.Join(config.ClaudeConfigDir(), "jobs") }

func readJobState(short string) (JobState, error) {
	var js JobState
	data, err := os.ReadFile(filepath.Join(jobsDir(), short, "state.json"))
	if err != nil {
		return js, err
	}
	err = json.Unmarshal(data, &js)
	return js, err
}

// listAgentsFromFiles reads the roster + job state files directly.
func listAgentsFromFiles() []Agent {
	type roster struct {
		Workers map[string]struct {
			PID       int    `json:"pid"`
			SessionID string `json:"sessionId"`
			CWD       string `json:"cwd"`
		} `json:"workers"`
	}
	var r roster
	data, err := os.ReadFile(filepath.Join(config.ClaudeConfigDir(), "daemon", "roster.json"))
	if err != nil || json.Unmarshal(data, &r) != nil {
		return nil
	}
	var out []Agent
	for short, w := range r.Workers {
		a := Agent{Short: short, PID: w.PID, SessionID: w.SessionID, CWD: w.CWD, Kind: "background"}
		if js, err := readJobState(short); err == nil {
			a.Name = js.Name
			a.State = js.State
			a.WaitingFor = js.WaitingFor
			if a.Name == "" {
				a.Name = js.Intent
			}
		}
		out = append(out, a)
	}
	return out
}

// ActiveAgentsBySession returns the live daemon workers keyed by their
// Claude session ID, read from the roster files (fast; no subprocess).
func ActiveAgentsBySession() map[string]Agent {
	out := map[string]Agent{}
	for _, a := range listAgentsFromFiles() {
		if a.SessionID != "" {
			out[a.SessionID] = a
		}
	}
	return out
}

// StopAgent stops a background worker via `claude stop`. The conversation is
// kept and becomes resumable interactively.
func StopAgent(short string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "claude", "stop", short).Run()
}

// WaitAgentGone polls the roster until the session's worker disappears (or
// the timeout passes). Returns true if the worker is gone.
func WaitAgentGone(sessionID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := ActiveAgentsBySession()[sessionID]; !ok {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// StateFromJob maps a daemon job state onto ctmux's status model.
func StateFromJob(state string) (status.State, bool) {
	switch state {
	case "working":
		return status.Busy, true
	case "blocked":
		return status.NeedsInput, true
	case "done":
		return status.Idle, true
	case "failed":
		return status.Error, true
	case "stopped":
		return status.Exited, true
	}
	return status.Starting, false
}

// WatchJob monitors one job's state.json and reports changes. Returns a stop
// function.
func WatchJob(short string, onChange func(JobState)) (func(), error) {
	dir := filepath.Join(jobsDir(), short)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}
	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Base(ev.Name) != "state.json" {
					continue
				}
				if js, err := readJobState(short); err == nil {
					onChange(js)
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return func() { w.Close() }, nil
}
