package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HookType indicates when a hook runs relative to tool execution.
type HookType string

const (
	PreHook  HookType = "pre"
	PostHook HookType = "post"
)

// Hook defines a shell command that runs before or after a tool.
type Hook struct {
	Tool    string   `json:"tool"`    // tool name ("bash", "write", "*" for all)
	Type    HookType `json:"type"`    // "pre" or "post"
	Command string   `json:"command"` // shell command to run
}

// HookResult holds the output of a hook execution.
type HookResult struct {
	Hook   Hook
	Output string
	Err    error
}

// Registry manages tool execution hooks.
type Registry struct {
	hooks []Hook
}

// Load reads hooks from .gilgamesh/hooks.json or ~/.config/gilgamesh/hooks.json.
func Load() *Registry {
	r := &Registry{}

	paths := []string{
		".gilgamesh/hooks.json",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "gilgamesh", "hooks.json"))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hooks []Hook
		if err := json.Unmarshal(data, &hooks); err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid hooks file %s: %s\n", p, err)
			continue
		}
		r.hooks = append(r.hooks, hooks...)
		break // first found wins
	}

	return r
}

// Run executes all matching hooks for a tool and hook type.
// For pre hooks, if any hook returns an error, it signals the tool should not run.
// The toolArgs JSON and tool result (for post hooks) are passed as env vars.
func (r *Registry) Run(hookType HookType, toolName string, toolArgs json.RawMessage, toolResult string) []HookResult {
	var results []HookResult

	for _, h := range r.hooks {
		if h.Type != hookType {
			continue
		}
		if h.Tool != "*" && h.Tool != toolName {
			continue
		}

		cmd := exec.Command("bash", "-c", h.Command)
		cmd.Env = append(os.Environ(),
			"GILGAMESH_TOOL="+toolName,
			"GILGAMESH_HOOK_TYPE="+string(hookType),
			"GILGAMESH_ARGS="+string(toolArgs),
		)
		if toolResult != "" {
			// Truncate result for env var safety
			r := toolResult
			if len(r) > 4096 {
				r = r[:4096]
			}
			cmd.Env = append(cmd.Env, "GILGAMESH_RESULT="+r)
		}

		done := make(chan error, 1)
		var out []byte
		go func() {
			var err error
			out, err = cmd.CombinedOutput()
			done <- err
		}()

		select {
		case err := <-done:
			results = append(results, HookResult{
				Hook:   h,
				Output: strings.TrimSpace(string(out)),
				Err:    err,
			})
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			results = append(results, HookResult{
				Hook: h,
				Err:  fmt.Errorf("hook timed out after 10s"),
			})
		}
	}

	return results
}

// HasHooks returns true if any hooks are registered.
func (r *Registry) HasHooks() bool {
	return len(r.hooks) > 0
}
