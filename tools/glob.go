package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func GlobTool() *Tool {
	return &Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Returns file paths.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.go, src/*.ts)"}
			},
			"required": ["pattern"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Pattern string `json:"pattern"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			// Use find with shell globbing for recursive support
			cmd := exec.Command("bash", "-c", fmt.Sprintf("find . -path './%s' -type f 2>/dev/null | head -100 | sort", p.Pattern))
			out, err := cmd.Output()

			result := strings.TrimSpace(string(out))

			if result == "" {
				// Fallback: try as literal glob
				cmd2 := exec.Command("bash", "-c", fmt.Sprintf("ls -1 %s 2>/dev/null | head -100", p.Pattern))
				out2, _ := cmd2.Output()
				result = strings.TrimSpace(string(out2))
			}

			if result == "" {
				return "No files found.", nil
			}

			lines := strings.Split(result, "\n")
			if len(lines) >= 100 {
				result += fmt.Sprintf("\n... (showing first 100 of possibly more)")
			}

			if err != nil && result == "" {
				return "", fmt.Errorf("glob error: %w", err)
			}

			return result, nil
		},
	}
}
