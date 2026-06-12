package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitaldan/tcc/internal/config"
)

// historyMaxMessages caps how many conversation messages are rendered into
// a tab's scrollback backfill.
const historyMaxMessages = 200

// historyLine is the subset of a transcript line needed for rendering.
type historyLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// contentBlock is one element of an assistant message's content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

// RenderTranscript renders a session's conversation as plain terminal text
// (CRLF lines, light ANSI styling) for backfilling a tab's scrollback —
// used when attaching to a background agent, whose live view never repaints
// past conversation. Returns nil when there is nothing to show.
func RenderTranscript(sessionID string) []byte {
	matches, _ := filepath.Glob(filepath.Join(config.ClaudeConfigDir(), "projects", "*", sessionID+".jsonl"))
	for _, m := range matches {
		if out := renderTranscriptFile(m); out != nil {
			return out
		}
	}
	return nil
}

func renderTranscriptFile(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	const dim, bold, reset = "\x1b[2m", "\x1b[1m", "\x1b[0m"

	var msgs []string
	sc := bufio.NewScanner(io.LimitReader(f, convScanLimit))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var l historyLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.IsSidechain || l.Message == nil {
			continue
		}
		switch l.Type {
		case "user":
			// Real typed prompts are plain strings; tool results arrive as
			// arrays on user-typed lines and injected wrappers start with <.
			var s string
			if json.Unmarshal(l.Message.Content, &s) != nil {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" || strings.HasPrefix(s, "<") || strings.HasPrefix(s, "Caveat:") {
				continue
			}
			msgs = append(msgs, bold+"❯ "+reset+s)
		case "assistant":
			var blocks []contentBlock
			if json.Unmarshal(l.Message.Content, &blocks) != nil {
				continue
			}
			var parts []string
			for _, b := range blocks {
				switch b.Type {
				case "text":
					if t := strings.TrimSpace(b.Text); t != "" {
						parts = append(parts, "⏺ "+t)
					}
				case "tool_use":
					parts = append(parts, dim+"⏺ [tool: "+b.Name+"]"+reset)
				}
			}
			if len(parts) > 0 {
				msgs = append(msgs, strings.Join(parts, "\n"))
			}
		}
	}

	if len(msgs) == 0 {
		return nil
	}
	trimmed := false
	if len(msgs) > historyMaxMessages {
		msgs = msgs[len(msgs)-historyMaxMessages:]
		trimmed = true
	}

	var buf bytes.Buffer
	buf.WriteString(dim + "── conversation history (from transcript) ──" + reset + "\r\n")
	if trimmed {
		buf.WriteString(dim + fmt.Sprintf("── older messages omitted (showing last %d) ──", historyMaxMessages) + reset + "\r\n")
	}
	buf.WriteString("\r\n")
	for _, m := range msgs {
		buf.WriteString(strings.ReplaceAll(m, "\n", "\r\n"))
		buf.WriteString("\r\n\r\n")
	}
	buf.WriteString(dim + "── end of history · live view below ──" + reset + "\r\n")
	return buf.Bytes()
}
