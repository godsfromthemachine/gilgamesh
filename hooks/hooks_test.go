package hooks

import (
	"encoding/json"
	"os"
	"testing"
)

func TestEmptyRegistry(t *testing.T) {
	r := &Registry{}
	if r.HasHooks() {
		t.Error("empty registry should have no hooks")
	}
	results := r.Run(PreHook, "bash", json.RawMessage(`{}`), "")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRunPreHook(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "bash", Type: PreHook, Command: "echo pre-hook-ran"},
		},
	}

	if !r.HasHooks() {
		t.Error("HasHooks() should return true")
	}

	results := r.Run(PreHook, "bash", json.RawMessage(`{"command":"ls"}`), "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("hook error: %v", results[0].Err)
	}
	if results[0].Output != "pre-hook-ran" {
		t.Errorf("output = %q, want 'pre-hook-ran'", results[0].Output)
	}
}

func TestRunPostHook(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "read", Type: PostHook, Command: "echo post-done"},
		},
	}

	results := r.Run(PostHook, "read", json.RawMessage(`{}`), "file contents")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("hook error: %v", results[0].Err)
	}
}

func TestWildcardHook(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "*", Type: PreHook, Command: "echo wildcard"},
		},
	}

	// Should match any tool
	for _, tool := range []string{"bash", "read", "write", "grep"} {
		results := r.Run(PreHook, tool, json.RawMessage(`{}`), "")
		if len(results) != 1 {
			t.Errorf("wildcard hook did not match tool %q", tool)
		}
	}
}

func TestHookTypeFiltering(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "bash", Type: PreHook, Command: "echo pre"},
			{Tool: "bash", Type: PostHook, Command: "echo post"},
		},
	}

	preResults := r.Run(PreHook, "bash", json.RawMessage(`{}`), "")
	if len(preResults) != 1 {
		t.Errorf("expected 1 pre-hook result, got %d", len(preResults))
	}

	postResults := r.Run(PostHook, "bash", json.RawMessage(`{}`), "result")
	if len(postResults) != 1 {
		t.Errorf("expected 1 post-hook result, got %d", len(postResults))
	}
}

func TestHookToolFiltering(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "bash", Type: PreHook, Command: "echo bash-only"},
		},
	}

	results := r.Run(PreHook, "read", json.RawMessage(`{}`), "")
	if len(results) != 0 {
		t.Errorf("hook for 'bash' should not match 'read'")
	}
}

func TestHookFailure(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "bash", Type: PreHook, Command: "exit 1"},
		},
	}

	results := r.Run(PreHook, "bash", json.RawMessage(`{}`), "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error for failing hook")
	}
}

func TestHookEnvironmentVars(t *testing.T) {
	r := &Registry{
		hooks: []Hook{
			{Tool: "read", Type: PreHook, Command: "echo $GILGAMESH_TOOL"},
		},
	}

	results := r.Run(PreHook, "read", json.RawMessage(`{"path":"test.go"}`), "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Output != "read" {
		t.Errorf("GILGAMESH_TOOL = %q, want 'read'", results[0].Output)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(".gilgamesh", 0755)
	hooksJSON := `[{"tool":"bash","type":"pre","command":"echo loaded"}]`
	os.WriteFile(".gilgamesh/hooks.json", []byte(hooksJSON), 0644)

	r := Load()
	if !r.HasHooks() {
		t.Error("Load() should have loaded hooks from file")
	}
}

func TestLoadNoFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	r := Load()
	if r.HasHooks() {
		t.Error("Load() should return empty registry when no hooks file exists")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll(".gilgamesh", 0755)
	os.WriteFile(".gilgamesh/hooks.json", []byte(`{invalid`), 0644)

	r := Load()
	if r.HasHooks() {
		t.Error("Load() should return empty registry for invalid JSON")
	}
}
