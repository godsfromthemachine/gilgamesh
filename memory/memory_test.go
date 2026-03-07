package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ".gilgamesh", "memory.json"))
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	if len(s.Entries) != 0 {
		t.Errorf("new store should have 0 entries, got %d", len(s.Entries))
	}
}

func TestAddAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gilgamesh", "memory.json")
	s := NewStore(path)

	s.Add("This project uses Go 1.25")
	s.Add("Always run tests before committing")

	if len(s.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s.Entries))
	}
	if s.Entries[0].Content != "This project uses Go 1.25" {
		t.Errorf("entry 0 = %q", s.Entries[0].Content)
	}
	if s.Entries[1].Content != "Always run tests before committing" {
		t.Errorf("entry 1 = %q", s.Entries[1].Content)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gilgamesh", "memory.json")

	// Save
	s := NewStore(path)
	s.Add("fact one")
	s.Add("fact two")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("memory file not created: %v", err)
	}

	// Load into new store
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s2.Entries) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(s2.Entries))
	}
	if s2.Entries[0].Content != "fact one" {
		t.Errorf("entry 0 = %q, want 'fact one'", s2.Entries[0].Content)
	}
}

func TestLoadNonexistent(t *testing.T) {
	s := NewStore("/tmp/nonexistent-gilgamesh-test/memory.json")
	err := s.Load()
	if err != nil {
		t.Errorf("Load on nonexistent file should return nil, got %v", err)
	}
	if len(s.Entries) != 0 {
		t.Errorf("entries should be empty after loading nonexistent file")
	}
}

func TestRemoveByIndex(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("first")
	s.Add("second")
	s.Add("third")

	if !s.Remove(1) {
		t.Error("Remove(1) should return true")
	}
	if len(s.Entries) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(s.Entries))
	}
	if s.Entries[0].Content != "first" || s.Entries[1].Content != "third" {
		t.Errorf("wrong entries after remove: %v", s.Entries)
	}
}

func TestRemoveOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("only")

	if s.Remove(-1) {
		t.Error("Remove(-1) should return false")
	}
	if s.Remove(1) {
		t.Error("Remove(1) should return false for 1-element store")
	}
	if len(s.Entries) != 1 {
		t.Error("store should still have 1 entry")
	}
}

func TestRemoveByContent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("use pytest")
	s.Add("deploy on port 8080")

	removed := s.RemoveByContent("pytest")
	if removed != 1 {
		t.Errorf("RemoveByContent returned %d, want 1", removed)
	}
	if len(s.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(s.Entries))
	}
	if s.Entries[0].Content != "deploy on port 8080" {
		t.Errorf("wrong remaining entry: %q", s.Entries[0].Content)
	}
}

func TestFormatForPrompt(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("Uses Go 1.25")
	s.Add("TDD approach")

	prompt := s.FormatForPrompt()
	if prompt == "" {
		t.Fatal("FormatForPrompt returned empty string")
	}
	if !strings.Contains(prompt, "Uses Go 1.25") {
		t.Error("prompt missing first entry")
	}
	if !strings.Contains(prompt, "TDD approach") {
		t.Error("prompt missing second entry")
	}
}

func TestFormatForPromptEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))

	prompt := s.FormatForPrompt()
	if prompt != "" {
		t.Errorf("empty store should return empty prompt, got %q", prompt)
	}
}

func TestFormatForPromptTruncation(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))

	// Add many entries to exceed ~200 token cap (~800 chars)
	for i := 0; i < 50; i++ {
		s.Add("This is a moderately long memory entry for testing truncation behavior")
	}

	prompt := s.FormatForPrompt()
	// Should be capped at roughly 800 chars (200 tokens * 4 chars/token)
	if len(prompt) > 1000 {
		t.Errorf("prompt too long (%d chars), should be truncated to ~800", len(prompt))
	}
}

func TestFormatList(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("fact one")
	s.Add("fact two")

	list := s.FormatList()
	if !strings.Contains(list, "1.") {
		t.Error("list should have numbered entries")
	}
	if !strings.Contains(list, "fact one") {
		t.Error("list missing entry")
	}
}

func TestFormatListEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))

	list := s.FormatList()
	if !strings.Contains(list, "No memories") {
		t.Errorf("empty list should say no memories, got %q", list)
	}
}

func TestDuplicatePrevention(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "mem.json"))
	s.Add("unique fact")
	s.Add("unique fact") // duplicate

	if len(s.Entries) != 1 {
		t.Errorf("expected 1 entry (duplicate prevented), got %d", len(s.Entries))
	}
}
