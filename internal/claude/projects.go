package claude

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/digitaldan/tcc/internal/config"
)

// maxSessions caps how many transcripts are peeked per listing.
const maxSessions = 200

// ListSessions discovers resumable sessions under the Claude config dir's
// projects tree, newest first.
func ListSessions() []ResumableSession {
	root := filepath.Join(config.ClaudeConfigDir(), "projects")

	type candidate struct {
		path string
		info os.FileInfo
	}
	var cands []candidate

	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
				continue
			}
			info, err := f.Info()
			if err != nil || info.Size() == 0 {
				continue
			}
			cands = append(cands, candidate{
				path: filepath.Join(root, d.Name(), f.Name()),
				info: info,
			})
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		return cands[i].info.ModTime().After(cands[j].info.ModTime())
	})
	if len(cands) > maxSessions {
		cands = cands[:maxSessions]
	}

	var out []ResumableSession
	for _, c := range cands {
		rs, ok := PeekSession(c.path)
		if !ok {
			continue
		}
		if rs.SessionID == "" {
			// Fall back to the filename, which is the session UUID.
			rs.SessionID = trimExt(filepath.Base(c.path))
		}
		rs.Modified = c.info.ModTime()
		out = append(out, rs)
	}
	return out
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

// TranscriptTitle finds the transcript for a session ID anywhere under the
// projects tree and returns its title ("" if not found).
func TranscriptTitle(sessionID string) string {
	matches, _ := filepath.Glob(filepath.Join(config.ClaudeConfigDir(), "projects", "*", sessionID+".jsonl"))
	for _, m := range matches {
		if rs, ok := PeekSession(m); ok {
			return rs.Title
		}
	}
	return ""
}
