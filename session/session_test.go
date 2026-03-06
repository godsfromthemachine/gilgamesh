package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	l := NewLogger()
	defer l.Close()

	if l.file == nil {
		t.Fatal("NewLogger returned logger with nil file")
	}
	p := l.Path()
	if p == "" {
		t.Fatal("Path() returned empty string")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("session file does not exist: %s", p)
	}
}

func TestLogAndRead(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	l := NewLogger()

	l.Log(Entry{Type: "user", Content: "hello"})
	l.Log(Entry{Type: "assistant", Content: "hi there"})
	l.Log(Entry{Type: "tool_call", Tool: "read", Args: json.RawMessage(`{"path":"main.go"}`)})
	l.Log(Entry{Type: "tool_result", Tool: "read", Content: "package main", Duration: 42 * time.Millisecond})

	l.Close()

	// Read back the file
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	// Verify first entry
	var e Entry
	json.Unmarshal([]byte(lines[0]), &e)
	if e.Type != "user" || e.Content != "hello" {
		t.Errorf("first entry: type=%q content=%q", e.Type, e.Content)
	}
	if e.Timestamp.IsZero() {
		t.Error("timestamp should be set automatically")
	}
}

func TestLogNilFile(t *testing.T) {
	l := &Logger{} // no file
	l.Log(Entry{Type: "user", Content: "should not panic"})
	l.Close() // should not panic
	if l.Path() != "" {
		t.Errorf("Path() should be empty for nil-file logger")
	}
}

func TestDistill(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	l := NewLogger()
	l.Log(Entry{Type: "user", Content: "list files"})
	l.Log(Entry{Type: "tool_call", Tool: "glob"})
	l.Log(Entry{Type: "tool_result", Tool: "glob", Content: "main.go"})
	l.Log(Entry{Type: "tool_call", Tool: "read"})
	l.Log(Entry{Type: "user", Content: "refactor this"})
	l.Log(Entry{Type: "tool_call", Tool: "edit"})
	l.Close()

	summary, err := Distill(l.Path())
	if err != nil {
		t.Fatalf("Distill error: %v", err)
	}

	if !containsStr(summary, "2 user messages") {
		t.Errorf("summary missing user message count: %s", summary)
	}
	if !containsStr(summary, "3 tool calls") {
		t.Errorf("summary missing tool call count: %s", summary)
	}
}

func TestDistillMissingFile(t *testing.T) {
	_, err := Distill("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one", 1},
		{"one\ntwo", 2},
		{"one\ntwo\n", 2},
		{"one\n\ntwo", 2}, // empty lines skipped
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestNewLoggerCreatesDir(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// .gilgamesh/sessions/ should not exist yet
	sessionDir := filepath.Join(dir, ".gilgamesh", "sessions")
	if _, err := os.Stat(sessionDir); err == nil {
		t.Fatal("sessions dir should not exist before NewLogger")
	}

	l := NewLogger()
	defer l.Close()

	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("NewLogger should create sessions dir: %v", err)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
