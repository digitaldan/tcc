package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHasConversation(t *testing.T) {
	// Metadata-only transcript: not a conversation.
	meta := writeTranscript(t,
		`{"type":"ai-title","aiTitle":"t","sessionId":"s"}`,
		`{"type":"file-history-snapshot"}`,
	)
	if HasConversation(meta) {
		t.Fatal("metadata-only transcript reported as conversation")
	}

	// Normal conversation.
	conv := writeTranscript(t,
		`{"type":"ai-title","aiTitle":"t","sessionId":"s"}`,
		`{"type":"user","message":{"content":"hi"},"cwd":"/tmp","sessionId":"s"}`,
		`{"type":"assistant","cwd":"/tmp","sessionId":"s"}`,
	)
	if !HasConversation(conv) {
		t.Fatal("conversation transcript not detected")
	}

	// A metadata line far larger than the read buffer must not abort the
	// scan before the conversation line (the old Scanner hard-stopped).
	huge := `{"type":"file-history-snapshot","blob":"` + strings.Repeat("x", 300*1024) + `"}`
	withHuge := writeTranscript(t,
		huge,
		`{"type":"user","message":{"content":"hi"},"cwd":"/tmp","sessionId":"s"}`,
	)
	if !HasConversation(withHuge) {
		t.Fatal("huge metadata line aborted conversation detection")
	}

	// A conversation line that itself exceeds the buffer is matched by
	// prefix.
	hugeUser := `{"type":"user","message":{"content":"` + strings.Repeat("y", 300*1024) + `"}}`
	if !HasConversation(writeTranscript(t, hugeUser)) {
		t.Fatal("oversized user line not detected via prefix")
	}

	// Missing file.
	if HasConversation(filepath.Join(t.TempDir(), "nope.jsonl")) {
		t.Fatal("missing file reported as conversation")
	}
}
