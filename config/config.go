package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
