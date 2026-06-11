package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/dcunningham/ctmux/internal/config"
	"github.com/dcunningham/ctmux/internal/status"
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
}

// ListAgents returns the daemon's background sessions, preferring the CLI
// surface (`claude agents --json`) and falling back to roster + job files.
func ListAgents() []Agent {
	if agents, err := listAgentsCLI(); err == nil {
		out := agents[:0]
		for _, a := range agents {
			if a.Kind == "background" {
				out = append(out, a)
			}
		}
		return out
	}
	return listAgentsFromFiles()
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
