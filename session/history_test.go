package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godsfromthemachine/gilgamesh/llm"
)

func TestSaveAndLoadHistory(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "2026-03-07-120000.jsonl")

	history := []llm.Message{
		{Role: "system", Content: "You are a helper."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	path, err := SaveHistory(sessionPath, history)
	if err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}
	if !strings.HasSuffix(path, ".history.json") {
		t.Errorf("path should end with .history.json, got %q", path)
	}

	loaded, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("loaded %d messages, want 3", len(loaded))
	}
	if loaded[0].Role != "system" {
		t.Errorf("msg 0 role = %q, want system", loaded[0].Role)
	}
	if loaded[1].Content != "Hello" {
		t.Errorf("msg 1 content = %q, want Hello", loaded[1].Content)
	}
	if loaded[2].Content != "Hi there!" {
		t.Errorf("msg 2 content = %q, want 'Hi there!'", loaded[2].Content)
	}
}

func TestSaveHistoryEmptyPath(t *testing.T) {
	path, err := SaveHistory("", nil)
	if err != nil {
		t.Fatalf("SaveHistory with empty path should not error: %v", err)
	}
	if path != "" {
		t.Errorf("path should be empty, got %q", path)
	}
}

func TestLoadHistoryNonexistent(t *testing.T) {
	_, err := LoadHistory("/tmp/nonexistent-gilgamesh-test/history.json")
	if err == nil {
		t.Error("LoadHistory should error on nonexistent file")
	}
}

func TestSaveHistoryWithToolCalls(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")

	history := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "list files"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "glob", Arguments: `{"pattern":"*.go"}`}},
		}},
		{Role: "tool", Content: "main.go\nagent.go", ToolCallID: "call_1"},
		{Role: "assistant", Content: "Found 2 Go files."},
	}

	path, err := SaveHistory(sessionPath, history)
	if err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}

	loaded, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(loaded) != 5 {
		t.Fatalf("loaded %d messages, want 5", len(loaded))
	}
	if len(loaded[2].ToolCalls) != 1 {
		t.Fatalf("msg 2 should have 1 tool call, got %d", len(loaded[2].ToolCalls))
	}
	if loaded[2].ToolCalls[0].Function.Name != "glob" {
		t.Errorf("tool call name = %q, want glob", loaded[2].ToolCalls[0].Function.Name)
	}
	if loaded[3].ToolCallID != "call_1" {
		t.Errorf("tool result ID = %q, want call_1", loaded[3].ToolCallID)
	}
}

func TestListHistories(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	sessDir := filepath.Join(dir, historyDir)
	os.MkdirAll(sessDir, 0755)

	// Create some history files
	for _, name := range []string{
		"2026-03-05-100000.history.json",
		"2026-03-06-100000.history.json",
		"2026-03-07-100000.history.json",
		"2026-03-07-100000.jsonl", // not a history file
	} {
		os.WriteFile(filepath.Join(sessDir, name), []byte("[]"), 0644)
	}

	histories := ListHistories(0)
	if len(histories) != 3 {
		t.Fatalf("expected 3 histories, got %d: %v", len(histories), histories)
	}
	// Should be newest first
	if !strings.Contains(histories[0], "03-07") {
		t.Errorf("first should be newest, got %q", histories[0])
	}

	// Test max limit
	limited := ListHistories(2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited histories, got %d", len(limited))
	}
}

func TestLatestHistory(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	sessDir := filepath.Join(dir, historyDir)
	os.MkdirAll(sessDir, 0755)

	os.WriteFile(filepath.Join(sessDir, "2026-03-05-100000.history.json"), []byte("[]"), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-03-07-140000.history.json"), []byte("[]"), 0644)

	latest := LatestHistory()
	if !strings.Contains(latest, "03-07-140000") {
		t.Errorf("latest should be 03-07-140000, got %q", latest)
	}
}

func TestLatestHistoryEmpty(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	latest := LatestHistory()
	if latest != "" {
		t.Errorf("latest should be empty when no histories, got %q", latest)
	}
}
