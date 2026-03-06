package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// detectLanguage checks for project files in the CWD to determine the test framework.
func detectLanguage() string {
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "go"},
		{"Cargo.toml", "rust"},
		{"build.zig", "zig"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{"pytest.ini", "python"},
		{"requirements.txt", "python"},
	}
	for _, c := range checks {
		if _, err := os.Stat(c.file); err == nil {
			return c.lang
		}
	}
	return "go" // default
}

func TestTool() *Tool {
	return &Tool{
		Name:        "test",
		Description: "Run tests. Auto-detects Go, Python, Rust, Zig, Node.js from project files. Returns output with pass/fail status.",
		Parameters: schema(`{
			"type": "object",
			"properties": {
				"language": {"type": "string", "description": "Language: go, python, rust, zig, node. Auto-detected if omitted."},
				"package":  {"type": "string", "description": "Package/path to test (e.g. ./..., ./pkg/foo, tests/). Default: all."},
				"run":      {"type": "string", "description": "Filter test names (go -run, pytest -k, cargo --test, etc.)"},
				"verbose":  {"type": "boolean", "description": "Verbose output"},
				"cover":    {"type": "boolean", "description": "Enable coverage (Go, Python)"},
				"short":    {"type": "boolean", "description": "Skip long tests (Go -short)"}
			}
		}`),
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Language string `json:"language"`
				Package  string `json:"package"`
				Run      string `json:"run"`
				Verbose  bool   `json:"verbose"`
				Cover    bool   `json:"cover"`
				Short    bool   `json:"short"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}

			lang := p.Language
			if lang == "" {
				lang = detectLanguage()
			}

			var program string
			var cmdArgs []string

			switch lang {
			case "go":
				program = "go"
				cmdArgs = []string{"test"}
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
				pkg := p.Package
				if pkg == "" {
					pkg = "./..."
				}
				cmdArgs = append(cmdArgs, pkg)

			case "python":
				program = "python3"
				cmdArgs = []string{"-m", "pytest"}
				if p.Verbose {
					cmdArgs = append(cmdArgs, "-v")
				}
				if p.Cover {
					cmdArgs = append(cmdArgs, "--cov")
				}
				if p.Run != "" {
					cmdArgs = append(cmdArgs, "-k", p.Run)
				}
				if p.Package != "" {
					cmdArgs = append(cmdArgs, p.Package)
				}

			case "rust":
				program = "cargo"
				cmdArgs = []string{"test"}
				if p.Verbose {
					cmdArgs = append(cmdArgs, "--verbose")
				}
				if p.Run != "" {
					cmdArgs = append(cmdArgs, "--test", p.Run)
				}
				if p.Package != "" {
					cmdArgs = append(cmdArgs, "-p", p.Package)
				}

			case "zig":
				program = "zig"
				cmdArgs = []string{"build", "test"}
				if p.Package != "" {
					// Zig test accepts a specific file
					cmdArgs = []string{"test", p.Package}
				}

			case "node":
				program = "npm"
				cmdArgs = []string{"test"}
				if p.Run != "" {
					cmdArgs = append(cmdArgs, "--", "--testNamePattern", p.Run)
				}

			default:
				return "", fmt.Errorf("unsupported language: %s (supported: go, python, rust, zig, node)", lang)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, program, cmdArgs...)
			out, err := cmd.CombinedOutput()

			result := string(out)
			if len(result) > 15000 {
				result = result[:7000] + "\n...(truncated)...\n" + result[len(result)-3000:]
			}

			if err != nil {
				return fmt.Sprintf("[%s] %s\n[FAIL: %s]", lang, result, err), nil
			}

			if strings.TrimSpace(result) == "" {
				return fmt.Sprintf("[%s] (no test output — no test files?)", lang), nil
			}
			return fmt.Sprintf("[%s] %s", lang, result), nil
		},
	}
}
