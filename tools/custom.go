package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const customToolTimeout = 120 * time.Second

// CustomToolDef defines a user-configured tool backed by a shell command.
type CustomToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Command     string          `json:"command"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// LoadCustomToolDefs reads custom tool definitions from .gilgamesh/tools.json.
// Returns nil if the file doesn't exist or is invalid.
func LoadCustomToolDefs() []CustomToolDef {
	data, err := os.ReadFile(".gilgamesh/tools.json")
	if err != nil {
		return nil
	}

	var defs []CustomToolDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil
	}

	// Filter out invalid entries
	valid := defs[:0]
	for _, d := range defs {
		if d.Name != "" && d.Command != "" {
			valid = append(valid, d)
		}
	}
	return valid
}

// RegisterCustom adds a custom tool to the registry.
func (r *Registry) RegisterCustom(def CustomToolDef) {
	params := def.Parameters
	if len(params) == 0 {
		params = schema(`{"type":"object","properties":{}}`)
	}

	t := &Tool{
		Name:        def.Name,
		Description: def.Description,
		Parameters:  params,
		Execute:     makeCustomExecutor(def.Command),
	}
	r.Register(t)
}

// makeCustomExecutor creates an executor function for a custom tool command.
// Args are available as:
//   - GILGAMESH_ARGS: full JSON args string
//   - GILGAMESH_<UPPER_KEY>: individual parameter values
//   - {{key}} in command: template substitution
func makeCustomExecutor(command string) func(args json.RawMessage) (string, error) {
	return func(args json.RawMessage) (string, error) {
		// Parse args for env vars and template substitution
		var argsMap map[string]interface{}
		json.Unmarshal(args, &argsMap)

		// Template substitution: replace {{key}} in command
		cmd := command
		env := os.Environ()
		env = append(env, "GILGAMESH_ARGS="+string(args))

		for key, val := range argsMap {
			valStr := fmt.Sprint(val)
			cmd = strings.ReplaceAll(cmd, "{{"+key+"}}", valStr)
			env = append(env, "GILGAMESH_"+strings.ToUpper(key)+"="+valStr)
		}

		ctx, cancel := context.WithTimeout(context.Background(), customToolTimeout)
		defer cancel()

		proc := exec.CommandContext(ctx, "bash", "-c", cmd)
		proc.Env = env

		var stdout, stderr bytes.Buffer
		proc.Stdout = &stdout
		proc.Stderr = &stderr

		if err := proc.Run(); err != nil {
			output := stderr.String()
			if output == "" {
				output = stdout.String()
			}
			return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(output))
		}

		return stdout.String(), nil
	}
}
