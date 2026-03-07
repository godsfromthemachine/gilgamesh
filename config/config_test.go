package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.ActiveModel != "default" {
		t.Errorf("ActiveModel = %q, want default", cfg.ActiveModel)
	}
	if len(cfg.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(cfg.Models))
	}
	for _, name := range []string{"fast", "default", "heavy"} {
		if _, ok := cfg.Models[name]; !ok {
			t.Errorf("missing model profile: %s", name)
		}
	}
}

func TestGetModel(t *testing.T) {
	cfg := DefaultConfig()

	m := cfg.GetModel()
	if m.Name != "qwen3.5-2b" {
		t.Errorf("default model name = %q, want qwen3.5-2b", m.Name)
	}

	cfg.ActiveModel = "heavy"
	m = cfg.GetModel()
	if m.Name != "qwen3.5-4b" {
		t.Errorf("heavy model name = %q, want qwen3.5-4b", m.Name)
	}
}

func TestGetModelFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ActiveModel = "nonexistent"
	m := cfg.GetModel()
	// Should fall back to "fast"
	if m.Endpoint != "http://127.0.0.1:8081/v1" {
		t.Errorf("fallback endpoint = %q, want 8081", m.Endpoint)
	}
}

func TestString(t *testing.T) {
	cfg := DefaultConfig()
	s := cfg.String()
	if s == "" {
		t.Error("String() returned empty")
	}
	if !contains(s, "default") || !contains(s, "qwen3.5-2b") {
		t.Errorf("String() missing expected content: %s", s)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	configJSON := `{
		"models": {
			"custom": {"name": "test-model", "endpoint": "http://localhost:9999/v1", "api_key": "test-key"}
		},
		"active_model": "custom"
	}`
	os.WriteFile("gilgamesh.json", []byte(configJSON), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ActiveModel != "custom" {
		t.Errorf("ActiveModel = %q, want custom", cfg.ActiveModel)
	}
	m := cfg.GetModel()
	if m.Name != "test-model" {
		t.Errorf("model name = %q, want test-model", m.Name)
	}
}

func TestLoadNoFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// Should return defaults
	if cfg.ActiveModel != "default" {
		t.Errorf("ActiveModel = %q, want default", cfg.ActiveModel)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.WriteFile("gilgamesh.json", []byte(`{invalid json`), 0644)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON config")
	}
}

func TestLoadHomeConfig(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Simulate home config
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	configDir := filepath.Join(home, ".config", "gilgamesh")
	configFile := filepath.Join(configDir, "gilgamesh.json")

	// Only test if the config file doesn't already exist
	if _, err := os.Stat(configFile); err == nil {
		t.Skip("home config already exists, skipping to avoid modification")
	}
}

func TestValidateGood(t *testing.T) {
	cfg := DefaultConfig()
	warnings := cfg.Validate()
	if len(warnings) != 0 {
		t.Errorf("default config should have no warnings, got %v", warnings)
	}
}

func TestValidateBadActiveModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ActiveModel = "nonexistent"
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		t.Error("expected warning for missing active_model")
	}
}

func TestValidateBadEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	m := cfg.Models["fast"]
	m.Endpoint = "ftp://bad"
	cfg.Models["fast"] = m
	warnings := cfg.Validate()
	found := false
	for _, w := range warnings {
		if contains(w, "http://") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about endpoint protocol")
	}
}

func TestValidateEmptyName(t *testing.T) {
	cfg := DefaultConfig()
	m := cfg.Models["fast"]
	m.Name = ""
	cfg.Models["fast"] = m
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		t.Error("expected warning for empty model name")
	}
}

func TestFormat(t *testing.T) {
	cfg := DefaultConfig()
	out := cfg.Format()
	if !contains(out, "* ") {
		t.Error("expected active model marker '*'")
	}
	if !contains(out, "default") {
		t.Error("expected 'default' in format output")
	}
}

func TestApplyEnv(t *testing.T) {
	cfg := DefaultConfig()

	t.Setenv("GILGAMESH_ACTIVE_MODEL", "heavy")
	t.Setenv("GILGAMESH_ENDPOINT", "http://override:9999/v1")
	t.Setenv("GILGAMESH_API_KEY", "sk-override")
	t.Setenv("GILGAMESH_MODEL_NAME", "override-model")

	cfg.ApplyEnv()

	if cfg.ActiveModel != "heavy" {
		t.Errorf("ActiveModel = %q, want heavy", cfg.ActiveModel)
	}
	m := cfg.GetModel()
	if m.Endpoint != "http://override:9999/v1" {
		t.Errorf("Endpoint = %q, want override", m.Endpoint)
	}
	if m.APIKey != "sk-override" {
		t.Errorf("APIKey = %q, want sk-override", m.APIKey)
	}
	if m.Name != "override-model" {
		t.Errorf("Name = %q, want override-model", m.Name)
	}
}

func TestApplyEnvNoVars(t *testing.T) {
	cfg := DefaultConfig()
	// Clear any that might be set
	for _, k := range []string{"GILGAMESH_ACTIVE_MODEL", "GILGAMESH_ENDPOINT", "GILGAMESH_API_KEY", "GILGAMESH_MODEL_NAME"} {
		os.Unsetenv(k)
	}
	cfg.ApplyEnv()
	if cfg.ActiveModel != "default" {
		t.Errorf("ActiveModel changed without env vars: %q", cfg.ActiveModel)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
