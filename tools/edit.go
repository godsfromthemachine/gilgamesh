package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func EditTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: "Find and replace text in a file. old_string must match exactly once.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"path":       {"type": "string", "description": "File path"},
				"old_string": {"type": "string", "description": "Text to find"},
				"new_string": {"type": "string", "description": "Replacement text"}
			},
			"required": ["path", "old_string", "new_string"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Path      string `json:"path"`
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			data, err := os.ReadFile(p.Path)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", p.Path, err)
			}

			content := string(data)
			count := strings.Count(content, p.OldString)
			if count == 0 {
				return "", fmt.Errorf("old_string not found in %s", p.Path)
			}
			if count > 1 {
				return "", fmt.Errorf("old_string found %d times in %s (must be unique)", count, p.Path)
			}

			newContent := strings.Replace(content, p.OldString, p.NewString, 1)
			if err := os.WriteFile(p.Path, []byte(newContent), 0644); err != nil {
				return "", fmt.Errorf("write: %w", err)
			}

			return fmt.Sprintf("Edited %s", p.Path), nil
		},
	}
}
