package config

import "github.com/spf13/viper"

type Config struct {
	Qdrant       QdrantConfig       `mapstructure:"qdrant"`
	RAG          RAGConfig          `mapstructure:"rag"`
	Embedding    EmbeddingConfig    `mapstructure:"embedding"`
	LLM          LLMConfig          `mapstructure:"llm"`
	Executor     ExecutorConfig     `mapstructure:"executor"`
	Conversation ConversationConfig `mapstructure:"conversation"`
	Logging      LoggingConfig      `mapstructure:"logging"`
}

type QdrantConfig struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	Collection string `mapstructure:"collection"`
}

type RAGConfig struct {
	TopK             int     `mapstructure:"top_k"`
	MinSimilarity    float32 `mapstructure:"min_similarity"`
	MaxContextLength int     `mapstructure:"max_context_length"`
}

type EmbeddingConfig struct {
	Endpoint string `mapstructure:"endpoint"`
}

type LLMConfig struct {
	Endpoint       string  `mapstructure:"endpoint"`
	Model          string  `mapstructure:"model"`
	Temperature    float32 `mapstructure:"temperature"`
	MaxTokens      int     `mapstructure:"max_tokens"`
	TimeoutSeconds int     `mapstructure:"timeout_seconds"`
}

type ExecutorConfig struct {
	DefaultStrategy     string `mapstructure:"default_strategy"`
	MaxRetries          int    `mapstructure:"max_retries"`
	RetryBackoffSeconds int    `mapstructure:"retry_backoff_seconds"`
}

type ConversationConfig struct {
	MaxMessages int `mapstructure:"max_messages"`
	MaxTokens   int `mapstructure:"max_tokens"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	
	return &cfg, nil
}