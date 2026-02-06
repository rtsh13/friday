package rag

import (
	"fmt"
	"sync"
)

// EmbeddingClient handles text embedding using ONNX runtime.
type EmbeddingClient struct {
	runtime   *ONNXRuntime
	tokenizer *BERTTokenizer
	batchSize int
	mu        sync.Mutex
}

// EmbeddingConfig holds configuration for the embedding client.
type EmbeddingConfig struct {
	ModelPath    string
	VocabPath    string
	MaxSeqLen    int
	EmbeddingDim int
	BatchSize    int
}

// DefaultEmbeddingConfig returns default embedding configuration.
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		ModelPath:    "./models/minilm-l6-v2.onnx",
		VocabPath:    "./models/vocab.json",
		MaxSeqLen:    128,
		EmbeddingDim: 384,
		BatchSize:    32,
	}
}

// NewEmbeddingClient creates a new ONNX-based embedding client.
func NewEmbeddingClient(cfg EmbeddingConfig) (*EmbeddingClient, error) {
	// Initialize tokenizer
	tokenizer, err := NewBERTTokenizer(cfg.VocabPath, cfg.MaxSeqLen)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenizer: %w", err)
	}

	// Initialize ONNX runtime
	runtime, err := NewONNXRuntime(cfg.ModelPath, cfg.EmbeddingDim, cfg.MaxSeqLen)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX runtime: %w", err)
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	return &EmbeddingClient{
		runtime:   runtime,
		tokenizer: tokenizer,
		batchSize: batchSize,
	}, nil
}

// Embed generates embeddings for a list of texts.
func (c *EmbeddingClient) Embed(texts []string) ([][]float32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(texts) == 0 {
		return nil, nil
	}

	// Process in batches
	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += c.batchSize {
		end := i + c.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := c.embedBatch(batch)
		if err != nil {
			return nil, fmt.Errorf("failed to embed batch %d: %w", i/c.batchSize, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)
	}

	return allEmbeddings, nil
}

// EmbedSingle generates embedding for a single text.
func (c *EmbeddingClient) EmbedSingle(text string) ([]float32, error) {
	embeddings, err := c.Embed([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}
	return embeddings[0], nil
}

// embedBatch processes a single batch of texts.
func (c *EmbeddingClient) embedBatch(texts []string) ([][]float32, error) {
	// Tokenize all texts
	tokenOutputs := c.tokenizer.EncodeBatch(texts)

	// Prepare input arrays
	inputIDs := make([][]int64, len(texts))
	attentionMasks := make([][]int64, len(texts))

	for i, output := range tokenOutputs {
		inputIDs[i] = output.InputIDs
		attentionMasks[i] = output.AttentionMask
	}

	// Run inference
	embeddings, err := c.runtime.Embed(inputIDs, attentionMasks)
	if err != nil {
		return nil, err
	}

	return embeddings, nil
}

// Close releases resources.
func (c *EmbeddingClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.runtime != nil {
		return c.runtime.Close()
	}
	return nil
}

// EmbeddingDim returns the embedding dimension.
func (c *EmbeddingClient) EmbeddingDim() int {
	return c.runtime.EmbeddingDim()
}

// VocabSize returns the tokenizer vocabulary size.
func (c *EmbeddingClient) VocabSize() int {
	return c.tokenizer.VocabSize()
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt32(normA) * sqrt32(normB))
}

// sqrt32 is a fast float32 square root.
func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson iteration
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
