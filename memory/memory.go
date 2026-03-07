package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxPromptChars caps memory injection into the system prompt (~200 tokens).
const maxPromptChars = 800

// Entry is a single remembered fact.
type Entry struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Store manages project-scoped memory entries.
type Store struct {
	Entries []Entry `json:"entries"`
	path    string
}

// NewStore creates a Store backed by the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads entries from disk. Returns nil if the file doesn't exist.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, s)
}

// Save writes entries to disk, creating directories as needed.
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Add appends a memory entry. Duplicates are silently ignored.
func (s *Store) Add(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	for _, e := range s.Entries {
		if e.Content == content {
			return
		}
	}
	s.Entries = append(s.Entries, Entry{
		Content:   content,
		CreatedAt: time.Now(),
	})
}

// Remove deletes the entry at the given index (0-based). Returns false if out of bounds.
func (s *Store) Remove(index int) bool {
	if index < 0 || index >= len(s.Entries) {
		return false
	}
	s.Entries = append(s.Entries[:index], s.Entries[index+1:]...)
	return true
}

// RemoveByContent removes all entries containing the given substring (case-insensitive).
// Returns the number of entries removed.
func (s *Store) RemoveByContent(substr string) int {
	lower := strings.ToLower(substr)
	removed := 0
	kept := s.Entries[:0]
	for _, e := range s.Entries {
		if strings.Contains(strings.ToLower(e.Content), lower) {
			removed++
		} else {
			kept = append(kept, e)
		}
	}
	s.Entries = kept
	return removed
}

// FormatForPrompt returns a compact string for system prompt injection.
// Truncates to stay under ~200 tokens (maxPromptChars characters).
// Returns empty string if no entries.
func (s *Store) FormatForPrompt() string {
	if len(s.Entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Memory:\n")

	for _, e := range s.Entries {
		line := "- " + e.Content + "\n"
		if sb.Len()+len(line) > maxPromptChars {
			break
		}
		sb.WriteString(line)
	}

	return sb.String()
}

// FormatList returns a numbered list for display.
func (s *Store) FormatList() string {
	if len(s.Entries) == 0 {
		return "No memories stored. Use /remember <fact> to add one."
	}

	var sb strings.Builder
	for i, e := range s.Entries {
		fmt.Fprintf(&sb, "  %d. %s\n", i+1, e.Content)
	}
	return sb.String()
}
