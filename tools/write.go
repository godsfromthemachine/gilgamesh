package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: "Create or overwrite a file.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"path":    {"type": "string", "description": "File path"},
				"content": {"type": "string", "description": "File content"}
			},
			"required": ["path", "content"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			if err := os.MkdirAll(filepath.Dir(p.Path), 0755); err != nil {
				return "", fmt.Errorf("mkdir: %w", err)
			}

			if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
				return "", fmt.Errorf("write: %w", err)
			}

			return fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), p.Path), nil
		},
	}
}
