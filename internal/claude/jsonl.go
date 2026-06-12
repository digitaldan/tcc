package claude

import (
	"bufio"
	"bytes"
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

// peekLine is the union of transcript line fields tcc cares about.
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
					s = strings.TrimSpace(s)
					// Skip injected wrappers (<local-command-caveat>,
					// <command-name>, …) — they aren't the user's words.
					if s != "" && !strings.HasPrefix(s, "<") && !strings.HasPrefix(s, "Caveat:") {
						firstUser = s
					}
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

// convScanLimit bounds how much of a transcript HasConversation examines.
// Generous because metadata lines (file-history snapshots) before the first
// message can be large.
const convScanLimit = 8 * 1024 * 1024

// HasConversation reports whether the session's transcript contains actual
// conversation messages — `claude --resume` refuses sessions that have only
// metadata lines (titles etc.). Oversized lines (huge tool results) are
// matched by prefix rather than aborting the scan.
func HasConversation(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	r := bufio.NewReaderSize(io.LimitReader(f, convScanLimit), 64*1024)
	for {
		line, err := r.ReadSlice('\n')
		if isConversationLine(line) {
			return true
		}
		switch err {
		case nil:
		case bufio.ErrBufferFull:
			// Huge line: the prefix was already checked; skip the rest.
			for err == bufio.ErrBufferFull {
				_, err = r.ReadSlice('\n')
			}
			if err != nil && err != bufio.ErrBufferFull {
				return false
			}
		default:
			return false // io.EOF or read error
		}
	}
}

// isConversationLine matches a transcript line that holds a user/assistant
// message. Tries full JSON first, falling back to a prefix match for lines
// that were truncated by the read buffer (claude writes the type field
// first).
func isConversationLine(line []byte) bool {
	var l peekLine
	if json.Unmarshal(line, &l) == nil {
		return l.Type == "user" || l.Type == "assistant"
	}
	return bytes.HasPrefix(line, []byte(`{"type":"user"`)) ||
		bytes.HasPrefix(line, []byte(`{"type":"assistant"`))
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
