package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	endpoint    string
	model       string
	temperature float32
	maxTokens   int
	client      *http.Client
}

func NewClient(endpoint, model string, timeout time.Duration, temperature float32, maxTokens int) *Client {
	return &Client{
		endpoint:    endpoint,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		client:      &http.Client{Timeout: timeout},
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	req := ChatRequest{
		Model: c.model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
<<<<<<< HEAD
		Temperature: 0.1,
		MaxTokens:   512,
=======
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
>>>>>>> 6de98cc314bf00a62b90597a7d943da55ad30a1f
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.endpoint+"/chat/completions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM returned status %d", resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode failed: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	return chatResp.Choices[0].Message.Content, nil
}
