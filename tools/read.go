package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func ReadTool() *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read file contents. Returns numbered lines.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"path":   {"type": "string", "description": "File path"},
				"offset": {"type": "integer", "description": "Start line (1-based)"},
				"limit":  {"type": "integer", "description": "Max lines to read"}
			},
			"required": ["path"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			data, err := os.ReadFile(p.Path)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", p.Path, err)
			}

			lines := strings.Split(string(data), "\n")

			start := 0
			if p.Offset > 0 {
				start = p.Offset - 1
			}
			if start > len(lines) {
				start = len(lines)
			}

			end := len(lines)
			if p.Limit > 0 && start+p.Limit < end {
				end = start + p.Limit
			}

			// Cap at 500 lines to avoid blowing context
			if end-start > 500 {
				end = start + 500
			}

			var b strings.Builder
			for i := start; i < end; i++ {
				fmt.Fprintf(&b, "%4d│%s\n", i+1, lines[i])
			}

			if end < len(lines) {
				fmt.Fprintf(&b, "... (%d more lines)\n", len(lines)-end)
			}

			return b.String(), nil
		},
	}
}
