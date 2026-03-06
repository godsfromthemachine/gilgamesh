package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func BashTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Execute a shell command. Returns stdout and stderr.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "Shell command to run"}
			},
			"required": ["command"]
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
			out, err := cmd.CombinedOutput()

			result := string(out)
			// Truncate very long output
			if len(result) > 10000 {
				result = result[:5000] + "\n...(truncated)...\n" + result[len(result)-2000:]
			}

			if err != nil {
				return fmt.Sprintf("%s\n[exit code: %s]", result, err), nil
			}

			if strings.TrimSpace(result) == "" {
				return "(no output)", nil
			}
			return result, nil
		},
	}
}
