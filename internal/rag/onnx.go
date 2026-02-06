package rag

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

// EmbeddingClient handles ONNX-based text embedding generation.
type EmbeddingClient struct {
	session    *ort.AdvancedSession
	tokenizer  *Tokenizer
	config     EmbeddingConfig
	inputNames []string
	outputName string
}

// EmbeddingConfig holds configuration for the embedding client.
type EmbeddingConfig struct {
	ModelPath     string
	TokenizerPath string
	MaxLength     int
	Dimension     int
}

// NewEmbeddingClient creates a new ONNX embedding client.
func NewEmbeddingClient(cfg EmbeddingConfig) (*EmbeddingClient, error) {
	// Initialize ONNX Runtime
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to initialize ONNX runtime: %w", err)
	}

	// Load tokenizer
	tokenizer, err := NewTokenizer(cfg.TokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}

	// Define input/output names for MiniLM model
	// Must match names used in scripts/01_export_onnx.py
	inputNames := []string{"input_ids", "attention_mask"}
	outputName := "output" // Matches Python export: output_names=["output"]

	// Create input tensors (will be replaced during inference)
	inputShape := ort.NewShape(1, int64(cfg.MaxLength))

	inputIdsTensor, err := ort.NewEmptyTensor[int64](inputShape)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}

	attentionMaskTensor, err := ort.NewEmptyTensor[int64](inputShape)
	if err != nil {
		inputIdsTensor.Destroy()
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}

	// Create output tensor
	outputShape := ort.NewShape(1, int64(cfg.MaxLength), int64(cfg.Dimension))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputIdsTensor.Destroy()
		attentionMaskTensor.Destroy()
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}

	// Create session options
	options, err := ort.NewSessionOptions()
	if err != nil {
		inputIdsTensor.Destroy()
		attentionMaskTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("failed to create session options: %w", err)
	}
	defer options.Destroy()

	// Create advanced session
	session, err := ort.NewAdvancedSession(
		cfg.ModelPath,
		inputNames,
		[]string{outputName},
		[]ort.ArbitraryTensor{inputIdsTensor, attentionMaskTensor},
		[]ort.ArbitraryTensor{outputTensor},
		options,
	)
	if err != nil {
		inputIdsTensor.Destroy()
		attentionMaskTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return &EmbeddingClient{
		session:    session,
		tokenizer:  tokenizer,
		config:     cfg,
		inputNames: inputNames,
		outputName: outputName,
	}, nil
}

// EmbedSingle generates an embedding for a single text.
func (c *EmbeddingClient) EmbedSingle(text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings generated")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (c *EmbeddingClient) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))

	for i, text := range texts {
		// Tokenize
		inputIds, attentionMask, err := c.tokenizer.Encode(text, c.config.MaxLength)
		if err != nil {
			return nil, fmt.Errorf("tokenization failed for text %d: %w", i, err)
		}

		// Create input tensors
		inputShape := ort.NewShape(1, int64(c.config.MaxLength))

		inputIdsTensor, err := ort.NewTensor(inputShape, inputIds)
		if err != nil {
			return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
		}

		attentionMaskTensor, err := ort.NewTensor(inputShape, attentionMask)
		if err != nil {
			inputIdsTensor.Destroy()
			return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
		}

		// Create output tensor
		outputShape := ort.NewShape(1, int64(c.config.MaxLength), int64(c.config.Dimension))
		outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
		if err != nil {
			inputIdsTensor.Destroy()
			attentionMaskTensor.Destroy()
			return nil, fmt.Errorf("failed to create output tensor: %w", err)
		}

		// Create session options for this inference
		options, err := ort.NewSessionOptions()
		if err != nil {
			inputIdsTensor.Destroy()
			attentionMaskTensor.Destroy()
			outputTensor.Destroy()
			return nil, fmt.Errorf("failed to create session options: %w", err)
		}

		// Create session for this inference
		session, err := ort.NewAdvancedSession(
			c.config.ModelPath,
			c.inputNames,
			[]string{c.outputName},
			[]ort.ArbitraryTensor{inputIdsTensor, attentionMaskTensor},
			[]ort.ArbitraryTensor{outputTensor},
			options,
		)
		if err != nil {
			options.Destroy()
			inputIdsTensor.Destroy()
			attentionMaskTensor.Destroy()
			outputTensor.Destroy()
			return nil, fmt.Errorf("failed to create ONNX session: %w", err)
		}

		// Run inference
		if err := session.Run(); err != nil {
			session.Destroy()
			options.Destroy()
			inputIdsTensor.Destroy()
			attentionMaskTensor.Destroy()
			outputTensor.Destroy()
			return nil, fmt.Errorf("ONNX inference failed: %w", err)
		}

		// Extract output and apply mean pooling
		outputData := outputTensor.GetData()
		embedding := meanPooling(outputData, attentionMask, c.config.MaxLength, c.config.Dimension)

		// Normalize embedding
		embedding = normalizeL2(embedding)

		results[i] = embedding

		// Cleanup
		session.Destroy()
		options.Destroy()
		inputIdsTensor.Destroy()
		attentionMaskTensor.Destroy()
		outputTensor.Destroy()
	}

	return results, nil
}

// Close releases resources held by the embedding client.
func (c *EmbeddingClient) Close() error {
	if c.session != nil {
		c.session.Destroy()
	}
	return nil
}

// meanPooling applies mean pooling over token embeddings using attention mask.
func meanPooling(output []float32, attentionMask []int64, seqLen, dim int) []float32 {
	result := make([]float32, dim)

	// Sum embeddings weighted by attention mask
	var totalWeight float32
	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 1 {
			for j := 0; j < dim; j++ {
				result[j] += output[i*dim+j]
			}
			totalWeight++
		}
	}

	// Average
	if totalWeight > 0 {
		for j := 0; j < dim; j++ {
			result[j] /= totalWeight
		}
	}

	return result
}

// normalizeL2 normalizes a vector to unit length.
func normalizeL2(v []float32) []float32 {
	var sum float64
	for _, val := range v {
		sum += float64(val) * float64(val)
	}

	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		return v
	}

	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = val / norm
	}

	return result
}
