// Package config handles cliche configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all cliche configuration.
type Config struct {
	// LLM settings
	OllamaURL   string `json:"ollama_url"`
	OllamaModel string `json:"ollama_model"`

	// Mode: "local" or "remote"
	Mode string `json:"mode"`

	// Privacy settings
	RedactSecrets bool `json:"redact_secrets"`
	Telemetry     bool `json:"telemetry"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		OllamaURL:     "http://localhost:11434",
		OllamaModel:   "qwen2.5:7b",
		Mode:          "local",
		RedactSecrets: true,
		Telemetry:     false,
	}
}

// ConfigDir returns the configuration directory path.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".cliche"), nil
}

// ConfigPath returns the configuration file path.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load loads configuration from disk, returning defaults if not found.
func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// Save saves configuration to disk.
func (c Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// Exists returns true if the config file exists.
func Exists() bool {
	path, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
