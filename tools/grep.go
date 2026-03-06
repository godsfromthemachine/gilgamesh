package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func GrepTool() *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search file contents for a pattern. Returns matching lines with file:line prefix.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Regex pattern to search"},
				"path":    {"type": "string", "description": "Directory or file to search (default: .)"},
				"include": {"type": "string", "description": "File glob filter (e.g. *.go)"}
			},
			"required": ["pattern"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
				Include string `json:"include"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			if p.Path == "" {
				p.Path = "."
			}

			cmdArgs := []string{"-rn", "--color=never"}
			if p.Include != "" {
				cmdArgs = append(cmdArgs, "--include="+p.Include)
			}
			cmdArgs = append(cmdArgs, p.Pattern, p.Path)

			cmd := exec.Command("grep", cmdArgs...)
			out, err := cmd.Output()

			result := string(out)
			if len(result) > 8000 {
				lines := strings.Split(result, "\n")
				if len(lines) > 50 {
					result = strings.Join(lines[:50], "\n") + fmt.Sprintf("\n... (%d more matches)", len(lines)-50)
				}
			}

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
					return "No matches found.", nil
				}
				return "", fmt.Errorf("grep error: %w", err)
			}

			if strings.TrimSpace(result) == "" {
				return "No matches found.", nil
			}
			return result, nil
		},
	}
}
