package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ModelConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
}

type Config struct {
	Models       map[string]ModelConfig `json:"models"`
	ActiveModel  string                 `json:"active_model"`
	AllowedTools []string               `json:"allowed_tools,omitempty"` // whitelist (if set, only these tools)
	DeniedTools  []string               `json:"denied_tools,omitempty"`  // blacklist (excluded tools)
}

func DefaultConfig() *Config {
	return &Config{
		Models: map[string]ModelConfig{
			"fast": {
				Name:     "qwen3.5-2b",
				Endpoint: "http://127.0.0.1:8081/v1",
				APIKey:   "sk-local",
			},
			"default": {
				Name:     "qwen3.5-2b",
				Endpoint: "http://127.0.0.1:8081/v1",
				APIKey:   "sk-local",
			},
			"heavy": {
				Name:     "qwen3.5-4b",
				Endpoint: "http://127.0.0.1:8080/v1",
				APIKey:   "sk-local",
			},
		},
		ActiveModel: "default",
	}
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Try loading from CWD first, then home config dir
	paths := []string{
		"gilgamesh.json",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "gilgamesh", "gilgamesh.json"))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		break
	}

	cfg.ApplyEnv()

	return cfg, nil
}

func (c *Config) String() string {
	m := c.GetModel()
	return fmt.Sprintf("Active: %s (%s) @ %s", c.ActiveModel, m.Name, m.Endpoint)
}

func (c *Config) GetModel() ModelConfig {
	if m, ok := c.Models[c.ActiveModel]; ok {
		return m
	}
	return c.Models["fast"]
}

// Validate checks the config for common errors and returns warnings.
func (c *Config) Validate() []string {
	var warnings []string

	if _, ok := c.Models[c.ActiveModel]; !ok {
		warnings = append(warnings, fmt.Sprintf("active_model %q not found in models", c.ActiveModel))
	}

	for name, m := range c.Models {
		if m.Name == "" {
			warnings = append(warnings, fmt.Sprintf("model %q has empty name", name))
		}
		if m.Endpoint == "" {
			warnings = append(warnings, fmt.Sprintf("model %q has empty endpoint", name))
		} else if !strings.HasPrefix(m.Endpoint, "http://") && !strings.HasPrefix(m.Endpoint, "https://") {
			warnings = append(warnings, fmt.Sprintf("model %q endpoint should start with http:// or https://", name))
		}
	}

	return warnings
}

// Format returns a multi-line summary of all model profiles.
func (c *Config) Format() string {
	var b strings.Builder
	for name, m := range c.Models {
		marker := "  "
		if name == c.ActiveModel {
			marker = "* "
		}
		fmt.Fprintf(&b, "%s%-10s %s @ %s\n", marker, name, m.Name, m.Endpoint)
	}
	return b.String()
}

// ApplyEnv overrides config values from environment variables.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("GILGAMESH_ACTIVE_MODEL"); v != "" {
		c.ActiveModel = v
	}
	if v := os.Getenv("GILGAMESH_ENDPOINT"); v != "" {
		m := c.GetModel()
		m.Endpoint = v
		c.Models[c.ActiveModel] = m
	}
	if v := os.Getenv("GILGAMESH_API_KEY"); v != "" {
		m := c.GetModel()
		m.APIKey = v
		c.Models[c.ActiveModel] = m
	}
	if v := os.Getenv("GILGAMESH_MODEL_NAME"); v != "" {
		m := c.GetModel()
		m.Name = v
		c.Models[c.ActiveModel] = m
	}
}
