package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a single logged event in a session.
type Entry struct {
	Timestamp time.Time       `json:"ts"`
	Type      string          `json:"type"` // "user", "assistant", "tool_call", "tool_result"
	Content   string          `json:"content,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Args      json.RawMessage `json:"args,omitempty"`
	Duration  time.Duration   `json:"duration_ms,omitempty"`
}

// Logger writes session events to a JSONL file.
type Logger struct {
	file *os.File
	enc  *json.Encoder
}

// NewLogger creates a session logger. Writes to .gilgamesh/sessions/<timestamp>.jsonl.
func NewLogger() *Logger {
	dir := ".gilgamesh/sessions"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Logger{} // silent fail — logging is best-effort
	}

	name := time.Now().Format("2006-01-02-150405") + ".jsonl"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return &Logger{}
	}

	return &Logger{file: f, enc: json.NewEncoder(f)}
}

// Log writes an entry to the session file.
func (l *Logger) Log(e Entry) {
	if l.file == nil {
		return
	}
	e.Timestamp = time.Now()
	if e.Duration > 0 {
		e.Duration = e.Duration / time.Millisecond
	}
	l.enc.Encode(e)
}

// Close flushes and closes the session file.
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// Path returns the path to the session log file.
func (l *Logger) Path() string {
	if l.file == nil {
		return ""
	}
	return l.file.Name()
}

// Distill reads a session JSONL file and returns a summary of actions taken.
func Distill(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var summary string
	var entries []Entry
	for _, line := range splitLines(string(data)) {
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}

	toolCalls := 0
	userMsgs := 0
	for _, e := range entries {
		switch e.Type {
		case "user":
			userMsgs++
		case "tool_call":
			toolCalls++
		}
	}

	summary = fmt.Sprintf("Session: %d user messages, %d tool calls", userMsgs, toolCalls)
	if len(entries) > 0 {
		summary += fmt.Sprintf(", duration: %s", entries[len(entries)-1].Timestamp.Sub(entries[0].Timestamp).Round(time.Second))
	}

	return summary, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
