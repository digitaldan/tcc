package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// ResumableSession is a past Claude Code session discovered from a
// transcript file under ~/.claude/projects/.
type ResumableSession struct {
	SessionID  string
	Path       string // transcript file
	Dir        string // cwd recorded inside the transcript (never decoded from the dir name)
	GitBranch  string
	Title      string
	Background bool // sessionKind == "bg"
	Modified   time.Time
}

// peekLine is the union of transcript line fields ctmux cares about.
type peekLine struct {
	Type        string `json:"type"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	CWD         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	SessionID   string `json:"sessionId"`
	SessionKind string `json:"sessionKind"`
	IsSidechain bool   `json:"isSidechain"`
	Summary     string `json:"summary"`
	Message     *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// peekLimit bounds how much of a transcript is read for metadata.
const peekLimit = 256 * 1024

// PeekSession reads the head of a transcript and extracts metadata. It
// returns ok=false for transcripts with nothing worth showing (no title and
// no user message).
func PeekSession(path string) (ResumableSession, bool) {
	rs := ResumableSession{Path: path}

	f, err := os.Open(path)
	if err != nil {
		return rs, false
	}
	defer f.Close()

	var firstUser string
	sc := bufio.NewScanner(io.LimitReader(f, peekLimit))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var l peekLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.SessionID != "" && rs.SessionID == "" {
			rs.SessionID = l.SessionID
		}
		switch l.Type {
		case "custom-title":
			if l.CustomTitle != "" {
				rs.Title = l.CustomTitle // user-set titles win
			}
		case "ai-title":
			if rs.Title == "" && l.AITitle != "" {
				rs.Title = l.AITitle
			}
		case "summary":
			if rs.Title == "" && l.Summary != "" {
				rs.Title = l.Summary
			}
		case "user":
			if l.IsSidechain {
				continue
			}
			if firstUser == "" && l.Message != nil {
				var s string
				if json.Unmarshal(l.Message.Content, &s) == nil {
					firstUser = strings.TrimSpace(s)
				}
			}
		}
		if l.CWD != "" && rs.Dir == "" {
			rs.Dir = l.CWD
			rs.GitBranch = l.GitBranch
		}
		if l.SessionKind == "bg" {
			rs.Background = true
		}
	}

	if rs.Title == "" {
		rs.Title = truncate(firstUser, 60)
	}
	if rs.Title == "" || rs.Dir == "" {
		return rs, false
	}
	return rs, true
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
