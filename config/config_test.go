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

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
