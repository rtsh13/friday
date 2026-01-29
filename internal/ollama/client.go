// Package ollama provides a client for interacting with the Ollama LLM API.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles communication with the Ollama API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	model      string
}

// Config holds client configuration.
type Config struct {
	BaseURL string        // e.g., "http://localhost:11434" or remote endpoint
	Model   string        // e.g., "qwen2.5:7b"
	Timeout time.Duration // Request timeout
}

// DefaultConfig returns sensible defaults for local development.
func DefaultConfig() Config {
	return Config{
		BaseURL: "http://localhost:11434",
		Model:   "qwen2.5:7b",
		Timeout: 60 * time.Second,
	}
}

// NewClient creates a new Ollama client.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Client{
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// GenerateRequest is the request body for the /api/generate endpoint.
type GenerateRequest struct {
	Model   string           `json:"model"`
	Prompt  string           `json:"prompt"`
	System  string           `json:"system,omitempty"`
	Stream  bool             `json:"stream"`
	Options *GenerateOptions `json:"options,omitempty"`
}

// GenerateOptions controls generation parameters.
type GenerateOptions struct {
	Temperature float64  `json:"temperature,omitempty"`
	TopP        float64  `json:"top_p,omitempty"`
	TopK        int      `json:"top_k,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// GenerateResponse is the response from /api/generate.
type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`

	// Timing info (only present when done=true)
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// Generate sends a prompt to the LLM and returns the response.
func (c *Client) Generate(ctx context.Context, prompt, systemPrompt string) (*GenerateResponse, error) {
	req := GenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		System: systemPrompt,
		Stream: false,
		Options: &GenerateOptions{
			Temperature: 0.7,
			TopP:        0.9,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var genResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &genResp, nil
}

// ChatMessage represents a message in the chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for the /api/chat endpoint.
type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []ChatMessage    `json:"messages"`
	Stream   bool             `json:"stream"`
	Options  *GenerateOptions `json:"options,omitempty"`
}

// ChatResponse is the response from /api/chat.
type ChatResponse struct {
	Model     string      `json:"model"`
	Message   ChatMessage `json:"message"`
	Done      bool        `json:"done"`
	CreatedAt string      `json:"created_at"`

	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// Chat sends a conversation to the LLM using the chat format.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (*ChatResponse, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Options: &GenerateOptions{
			Temperature: 0.7,
			TopP:        0.9,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &chatResp, nil
}

// Ping checks if the Ollama server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	// httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	// if err != nil {
	// 	return fmt.Errorf("create request: %w", err)
	// }

	// resp, err := c.httpClient.Do(httpReq)
	// if err != nil {
	// 	return fmt.Errorf("ollama not reachable: %w", err)
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	// }

	return nil
}

// ListModels returns the available models.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}

	return names, nil
}

// ModelInfo returns information about the configured model.
func (c *Client) ModelInfo() string {
	return fmt.Sprintf("%s @ %s", c.model, c.baseURL)
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.model
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}
