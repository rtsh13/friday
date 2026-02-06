// Package config handles telemetry-debugger configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Qdrant       QdrantConfig       `mapstructure:"qdrant" yaml:"qdrant"`
	RAG          RAGConfig          `mapstructure:"rag" yaml:"rag"`
	ONNX         ONNXConfig         `mapstructure:"onnx" yaml:"onnx"`
	LLM          LLMConfig          `mapstructure:"llm" yaml:"llm"`
	Executor     ExecutorConfig     `mapstructure:"executor" yaml:"executor"`
	Conversation ConversationConfig `mapstructure:"conversation" yaml:"conversation"`
	UI           UIConfig           `mapstructure:"ui" yaml:"ui"`
	Logging      LoggingConfig      `mapstructure:"logging" yaml:"logging"`
}

// QdrantConfig holds vector database settings.
type QdrantConfig struct {
	Host       string `mapstructure:"host" yaml:"host"`
	Port       int    `mapstructure:"port" yaml:"port"`
	Collection string `mapstructure:"collection" yaml:"collection"`
}

// RAGConfig holds retrieval-augmented generation settings.
type RAGConfig struct {
	TopK             int     `mapstructure:"top_k" yaml:"top_k"`
	MinSimilarity    float32 `mapstructure:"min_similarity" yaml:"min_similarity"`
	MaxContextLength int     `mapstructure:"max_context_length" yaml:"max_context_length"`
}

// ONNXConfig holds ONNX embedding model settings.
type ONNXConfig struct {
	ModelPath         string `mapstructure:"model_path" yaml:"model_path"`
	VocabPath         string `mapstructure:"vocab_path" yaml:"vocab_path"`
	MaxSequenceLength int    `mapstructure:"max_sequence_length" yaml:"max_sequence_length"`
	EmbeddingDim      int    `mapstructure:"embedding_dim" yaml:"embedding_dim"`
}

// LLMConfig holds LLM (vLLM) settings.
type LLMConfig struct {
	Endpoint       string  `mapstructure:"endpoint" yaml:"endpoint"`
	Model          string  `mapstructure:"model" yaml:"model"`
	Temperature    float32 `mapstructure:"temperature" yaml:"temperature"`
	MaxTokens      int     `mapstructure:"max_tokens" yaml:"max_tokens"`
	TimeoutSeconds int     `mapstructure:"timeout_seconds" yaml:"timeout_seconds"`
}

// ExecutorConfig holds function execution settings.
type ExecutorConfig struct {
	DefaultStrategy     string `mapstructure:"default_strategy" yaml:"default_strategy"`
	MaxRetries          int    `mapstructure:"max_retries" yaml:"max_retries"`
	RetryBackoffSeconds int    `mapstructure:"retry_backoff_seconds" yaml:"retry_backoff_seconds"`
}

// ConversationConfig holds conversation context settings.
type ConversationConfig struct {
	MaxMessages int `mapstructure:"max_messages" yaml:"max_messages"`
	MaxTokens   int `mapstructure:"max_tokens" yaml:"max_tokens"`
}

// UIConfig holds UI settings.
type UIConfig struct {
	ShowToolOutput bool `mapstructure:"show_tool_output" yaml:"show_tool_output"`
	Verbose        bool `mapstructure:"verbose" yaml:"verbose"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Qdrant: QdrantConfig{
			Host:       "localhost",
			Port:       6333,
			Collection: "telemetry_docs",
		},
		RAG: RAGConfig{
			TopK:             5,
			MinSimilarity:    0.7,
			MaxContextLength: 4000,
		},
		ONNX: ONNXConfig{
			ModelPath:         "./models/minilm-l6-v2.onnx",
			VocabPath:         "./models/vocab.json",
			MaxSequenceLength: 128,
			EmbeddingDim:      384,
		},
		LLM: LLMConfig{
			Endpoint:       "http://localhost:8000/v1",
			Model:          "Qwen/Qwen2.5-7B-Instruct",
			Temperature:    0.1,
			MaxTokens:      2048,
			TimeoutSeconds: 60,
		},
		Executor: ExecutorConfig{
			DefaultStrategy:     "stop_on_error",
			MaxRetries:          2,
			RetryBackoffSeconds: 1,
		},
		Conversation: ConversationConfig{
			MaxMessages: 10,
			MaxTokens:   4000,
		},
		UI: UIConfig{
			ShowToolOutput: true,
			Verbose:        false,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load loads configuration from the specified YAML file.
// Falls back to defaults if file doesn't exist.
func Load(path string) (*Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	viper.SetConfigFile(path)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// LoadFromPaths tries to load config from multiple paths in order.
// Returns the first successfully loaded config.
func LoadFromPaths(paths ...string) (*Config, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return Load(path)
		}
	}
	// Return defaults if no config file found
	return DefaultConfig(), nil
}

// Save saves the configuration to a YAML file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// UserConfigDir returns the user-specific configuration directory.
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".telemetry-debugger"), nil
}

// UserConfigPath returns the user-specific configuration file path.
func UserConfigPath() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Validate validates the configuration values.
func (c *Config) Validate() error {
	if c.Qdrant.Host == "" {
		return fmt.Errorf("qdrant.host is required")
	}
	if c.Qdrant.Port <= 0 || c.Qdrant.Port > 65535 {
		return fmt.Errorf("qdrant.port must be between 1 and 65535")
	}
	if c.Qdrant.Collection == "" {
		return fmt.Errorf("qdrant.collection is required")
	}
	if c.ONNX.ModelPath == "" {
		return fmt.Errorf("onnx.model_path is required")
	}
	if c.ONNX.VocabPath == "" {
		return fmt.Errorf("onnx.vocab_path is required")
	}
	if c.ONNX.EmbeddingDim <= 0 {
		return fmt.Errorf("onnx.embedding_dim must be positive")
	}
	if c.LLM.Endpoint == "" {
		return fmt.Errorf("llm.endpoint is required")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	return nil
}
