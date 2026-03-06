package tools

import (
	"encoding/json"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}

	tools := r.Tools()
	expected := []string{"read", "write", "edit", "bash", "grep", "glob", "test"}
	if len(tools) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(tools))
	}

	for _, name := range expected {
		if _, ok := tools[name]; !ok {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestDefinitions(t *testing.T) {
	r := NewRegistry()
	defs := r.Definitions()

	if len(defs) != 7 {
		t.Fatalf("expected 7 definitions, got %d", len(defs))
	}

	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("tool %s: type = %q, want \"function\"", d.Function.Name, d.Type)
		}
		if d.Function.Name == "" {
			t.Error("tool definition has empty name")
		}
		if d.Function.Description == "" {
			t.Errorf("tool %s has empty description", d.Function.Name)
		}
		if len(d.Function.Parameters) == 0 {
			t.Errorf("tool %s has empty parameters", d.Function.Name)
		}
		// Verify parameters is valid JSON
		var params map[string]interface{}
		if err := json.Unmarshal(d.Function.Parameters, &params); err != nil {
			t.Errorf("tool %s: invalid parameters JSON: %v", d.Function.Name, err)
		}
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute("nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestReadTool(t *testing.T) {
	r := NewRegistry()
	// Read the registry file itself
	result, err := r.Execute("read", json.RawMessage(`{"path": "registry.go"}`))
	if err != nil {
		t.Fatalf("read registry.go: %v", err)
	}
	if result == "" {
		t.Fatal("read returned empty result")
	}
	// Should contain the package declaration
	if !containsStr(result, "package tools") {
		t.Error("read result missing 'package tools'")
	}
}

func TestReadToolMissing(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute("read", json.RawMessage(`{"path": "nonexistent_file_xyz.go"}`))
	if err == nil {
		t.Fatal("expected error reading nonexistent file")
	}
}

func TestGlobTool(t *testing.T) {
	r := NewRegistry()
	result, err := r.Execute("glob", json.RawMessage(`{"pattern": "*.go"}`))
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if !containsStr(result, "registry.go") {
		t.Error("glob result missing registry.go")
	}
}

func TestGrepTool(t *testing.T) {
	r := NewRegistry()
	result, err := r.Execute("grep", json.RawMessage(`{"pattern": "func NewRegistry", "path": "."}`))
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !containsStr(result, "registry.go") {
		t.Error("grep result missing registry.go")
	}
}

func TestRegisterCustomTool(t *testing.T) {
	r := NewRegistry()
	initial := len(r.Tools())

	r.Register(&Tool{
		Name:        "custom",
		Description: "A custom tool",
		Parameters:  schema(`{"type": "object"}`),
		Execute: func(args json.RawMessage) (string, error) {
			return "custom result", nil
		},
	})

	if len(r.Tools()) != initial+1 {
		t.Error("custom tool not registered")
	}

	result, err := r.Execute("custom", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("custom tool error: %v", err)
	}
	if result != "custom result" {
		t.Errorf("got %q, want %q", result, "custom result")
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
