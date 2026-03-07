package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/godsfromthemachine/gilgamesh/llm"
)

const historyDir = ".gilgamesh/sessions"
const historySuffix = ".history.json"

// SaveHistory writes the conversation history to a JSON file alongside the session log.
// Returns the path written, or error.
func SaveHistory(sessionPath string, history []llm.Message) (string, error) {
	if sessionPath == "" {
		return "", nil
	}

	// Derive history path from session log path: foo.jsonl → foo.history.json
	histPath := strings.TrimSuffix(sessionPath, ".jsonl") + historySuffix

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(histPath, data, 0644); err != nil {
		return "", err
	}
	return histPath, nil
}

// LoadHistory reads a conversation history from a JSON file.
func LoadHistory(path string) ([]llm.Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var history []llm.Message
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}

// LatestHistory returns the path to the most recent .history.json file, or empty if none.
func LatestHistory() string {
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		return ""
	}

	var histFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), historySuffix) {
			histFiles = append(histFiles, filepath.Join(historyDir, e.Name()))
		}
	}

	if len(histFiles) == 0 {
		return ""
	}

	sort.Strings(histFiles) // chronological by filename (timestamp-based)
	return histFiles[len(histFiles)-1]
}

// ListHistories returns the most recent N history files (newest first).
func ListHistories(max int) []string {
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		return nil
	}

	var histFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), historySuffix) {
			histFiles = append(histFiles, e.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(histFiles)))

	if max > 0 && len(histFiles) > max {
		histFiles = histFiles[:max]
	}
	return histFiles
}
