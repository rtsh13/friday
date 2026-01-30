package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type EmbeddingClient struct {
	endpoint string
	client   *http.Client
}

func NewEmbeddingClient(endpoint string) *EmbeddingClient {
	return &EmbeddingClient{
		endpoint: endpoint,
		client:   &http.Client{},
	}
}

type EmbeddingRequest struct {
	Inputs []string `json:"inputs"`
}

type EmbeddingResponse [][]float32

func (ec *EmbeddingClient) Embed(texts []string) ([][]float32, error) {
	req := EmbeddingRequest{Inputs: texts}
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	
	resp, err := ec.client.Post(
		ec.endpoint,
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}
	
	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return embResp, nil
}