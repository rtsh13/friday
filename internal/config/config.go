package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Ollama/LLM settings
	OllamaURL   string `json:"ollama_url" mapstructure:"ollama_url"`
	OllamaModel string `json:"ollama_model" mapstructure:"ollama_model"`
	Mode        string `json:"mode" mapstructure:"mode"` // "local" or "remote"

	// RAG and Vector DB
	Qdrant    QdrantConfig    `json:"qdrant" mapstructure:"qdrant"`
	RAG       RAGConfig       `json:"rag" mapstructure:"rag"`
	Embedding EmbeddingConfig `json:"embedding" mapstructure:"embedding"`

	// LLM advanced settings
	LLM LLMConfig `json:"llm" mapstructure:"llm"`

	// Execution and conversation
	Executor     ExecutorConfig     `json:"executor" mapstructure:"executor"`
	Conversation ConversationConfig `json:"conversation" mapstructure:"conversation"`

	// System settings
	Logging       LoggingConfig `json:"logging" mapstructure:"logging"`
	RedactSecrets bool          `json:"redact_secrets" mapstructure:"redact_secrets"`
	Telemetry     bool          `json:"telemetry" mapstructure:"telemetry"`
}

type QdrantConfig struct {
	Host       string `json:"host" mapstructure:"host"`
	Port       int    `json:"port" mapstructure:"port"`
	Collection string `json:"collection" mapstructure:"collection"`
}

type RAGConfig struct {
	TopK             int     `json:"top_k" mapstructure:"top_k"`
	MinSimilarity    float32 `json:"min_similarity" mapstructure:"min_similarity"`
	MaxContextLength int     `json:"max_context_length" mapstructure:"max_context_length"`
}

type EmbeddingConfig struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
}

type LLMConfig struct {
	Endpoint       string  `json:"endpoint" mapstructure:"endpoint"`
	Model          string  `json:"model" mapstructure:"model"`
	Temperature    float32 `json:"temperature" mapstructure:"temperature"`
	MaxTokens      int     `json:"max_tokens" mapstructure:"max_tokens"`
	TimeoutSeconds int     `json:"timeout_seconds" mapstructure:"timeout_seconds"`
}

type ExecutorConfig struct {
	DefaultStrategy     string `json:"default_strategy" mapstructure:"default_strategy"`
	MaxRetries          int    `json:"max_retries" mapstructure:"max_retries"`
	RetryBackoffSeconds int    `json:"retry_backoff_seconds" mapstructure:"retry_backoff_seconds"`
}

type ConversationConfig struct {
	MaxMessages int `json:"max_messages" mapstructure:"max_messages"`
	MaxTokens   int `json:"max_tokens" mapstructure:"max_tokens"`
}

type LoggingConfig struct {
	Level  string `json:"level" mapstructure:"level"`
	Format string `json:"format" mapstructure:"format"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		OllamaURL:     "http://localhost:11434",
		OllamaModel:   "qwen2.5:7b",
		Mode:          "local",
		RedactSecrets: true,
		Telemetry:     false,
		Qdrant: QdrantConfig{
			Host:       "localhost",
			Port:       6333,
			Collection: "documents",
		},
		RAG: RAGConfig{
			TopK:             5,
			MinSimilarity:    0.7,
			MaxContextLength: 2000,
		},
		Embedding: EmbeddingConfig{
			Endpoint: "http://localhost:8000",
		},
		LLM: LLMConfig{
			Endpoint:       "http://localhost:11434",
			Model:          "qwen2.5:7b",
			Temperature:    0.7,
			MaxTokens:      2048,
			TimeoutSeconds: 300,
		},
		Executor: ExecutorConfig{
			DefaultStrategy:     "sequential",
			MaxRetries:          3,
			RetryBackoffSeconds: 1,
		},
		Conversation: ConversationConfig{
			MaxMessages: 100,
			MaxTokens:   10000,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// ConfigDir returns the configuration directory path
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".cliche"), nil
}

// ConfigPath returns the configuration file path
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load loads configuration from disk
// Tries JSON first (user config), then falls back to Viper for other formats if configPath is provided
func Load(configPath ...string) (Config, error) {
	cfg := DefaultConfig()

	// Try loading from standard location first
	path, err := ConfigPath()
	if err == nil {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("parse config: %w", err)
			}
			return cfg, nil
		}
	}

	// If a custom path is provided, try Viper (supports YAML, TOML, etc.)
	if len(configPath) > 0 && configPath[0] != "" {
		viper.SetConfigFile(configPath[0])
		if err := viper.ReadInConfig(); err != nil {
			return cfg, fmt.Errorf("viper read config: %w", err)
		}
		if err := viper.Unmarshal(&cfg); err != nil {
			return cfg, fmt.Errorf("viper unmarshal config: %w", err)
		}
		return cfg, nil
	}

	// No config file found, return defaults
	return cfg, nil
}

// Save persists configuration to disk as JSON
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

// Exists returns true if the config file exists
func Exists() bool {
	path, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
