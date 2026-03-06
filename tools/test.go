package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func TestTool() *Tool {
	return &Tool{
		Name:        "test",
		Description: "Run Go tests. Can run all tests, specific packages, or specific test functions. Returns test output with pass/fail status.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"package": {"type": "string", "description": "Package path to test (e.g. ./..., ./pkg/foo). Default: ./..."},
				"run":     {"type": "string", "description": "Regex to filter test names (go test -run)"},
				"verbose": {"type": "boolean", "description": "Verbose output (go test -v)"},
				"cover":   {"type": "boolean", "description": "Enable coverage reporting"},
				"short":   {"type": "boolean", "description": "Skip long-running tests (go test -short)"}
			}
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Package string `json:"package"`
				Run     string `json:"run"`
				Verbose bool   `json:"verbose"`
				Cover   bool   `json:"cover"`
				Short   bool   `json:"short"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			if p.Package == "" {
				p.Package = "./..."
			}

			cmdArgs := []string{"test"}
			if p.Verbose {
				cmdArgs = append(cmdArgs, "-v")
			}
			if p.Cover {
				cmdArgs = append(cmdArgs, "-cover")
			}
			if p.Short {
				cmdArgs = append(cmdArgs, "-short")
			}
			if p.Run != "" {
				cmdArgs = append(cmdArgs, "-run", p.Run)
			}
			cmdArgs = append(cmdArgs, p.Package)

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", cmdArgs...)
			out, err := cmd.CombinedOutput()

			result := string(out)
			// Truncate very long test output
			if len(result) > 15000 {
				result = result[:7000] + "\n...(truncated)...\n" + result[len(result)-3000:]
			}

			if err != nil {
				// Test failures are expected — return output with status
				return fmt.Sprintf("%s\n[FAIL: %s]", result, err), nil
			}

			if strings.TrimSpace(result) == "" {
				return "(no test output — no test files?)", nil
			}
			return result, nil
		},
	}
}
