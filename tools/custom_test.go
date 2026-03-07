package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCustomToolsEmpty(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	defs := LoadCustomToolDefs()
	if len(defs) != 0 {
		t.Errorf("expected 0 custom tools, got %d", len(defs))
	}
}

func TestLoadCustomToolsFromFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(".gilgamesh", 0755)
	config := `[
		{
			"name": "greet",
			"description": "Say hello",
			"command": "echo hello"
		}
	]`
	os.WriteFile(filepath.Join(".gilgamesh", "tools.json"), []byte(config), 0644)

	defs := LoadCustomToolDefs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 custom tool, got %d", len(defs))
	}
	if defs[0].Name != "greet" {
		t.Errorf("name = %q, want greet", defs[0].Name)
	}
	if defs[0].Command != "echo hello" {
		t.Errorf("command = %q, want 'echo hello'", defs[0].Command)
	}
}

func TestCustomToolExecution(t *testing.T) {
	tool := CustomToolDef{
		Name:        "hello",
		Description: "Say hello",
		Command:     "echo hello world",
	}

	reg := &Registry{tools: make(map[string]*Tool)}
	reg.RegisterCustom(tool)

	result, err := reg.Execute("hello", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("result = %q, want 'hello world'", result)
	}
}

func TestCustomToolWithArgs(t *testing.T) {
	tool := CustomToolDef{
		Name:        "greet",
		Description: "Greet someone",
		Command:     "echo Hello $GILGAMESH_NAME",
	}

	reg := &Registry{tools: make(map[string]*Tool)}
	reg.RegisterCustom(tool)

	result, err := reg.Execute("greet", json.RawMessage(`{"name":"World"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Hello World") {
		t.Errorf("result = %q, want 'Hello World'", result)
	}
}

func TestCustomToolWithTemplate(t *testing.T) {
	tool := CustomToolDef{
		Name:        "cat_file",
		Description: "Read a file",
		Command:     "cat {{path}}",
	}

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("file contents"), 0644)

	reg := &Registry{tools: make(map[string]*Tool)}
	reg.RegisterCustom(tool)

	result, err := reg.Execute("cat_file", json.RawMessage(`{"path":"`+testFile+`"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "file contents") {
		t.Errorf("result = %q, want 'file contents'", result)
	}
}

func TestCustomToolError(t *testing.T) {
	tool := CustomToolDef{
		Name:        "fail",
		Description: "Always fails",
		Command:     "exit 1",
	}

	reg := &Registry{tools: make(map[string]*Tool)}
	reg.RegisterCustom(tool)

	_, err := reg.Execute("fail", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error from failing command")
	}
}

func TestCustomToolParameters(t *testing.T) {
	tool := CustomToolDef{
		Name:        "with_params",
		Description: "Has parameters",
		Command:     "echo ok",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"file":{"type":"string"}},"required":["file"]}`),
	}

	reg := &Registry{tools: make(map[string]*Tool)}
	reg.RegisterCustom(tool)

	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Function.Name == "with_params" {
			found = true
			if string(d.Function.Parameters) == "" {
				t.Error("parameters should not be empty")
			}
		}
	}
	if !found {
		t.Error("custom tool not found in definitions")
	}
}

func TestSkipInvalidCustomTools(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(".gilgamesh", 0755)
	// Missing name and command — should be skipped
	config := `[
		{"name": "", "description": "no name", "command": "echo hi"},
		{"name": "no_cmd", "description": "no command", "command": ""},
		{"name": "valid", "description": "good", "command": "echo ok"}
	]`
	os.WriteFile(filepath.Join(".gilgamesh", "tools.json"), []byte(config), 0644)

	defs := LoadCustomToolDefs()
	if len(defs) != 1 {
		t.Errorf("expected 1 valid custom tool, got %d", len(defs))
	}
	if defs[0].Name != "valid" {
		t.Errorf("name = %q, want valid", defs[0].Name)
	}
}
