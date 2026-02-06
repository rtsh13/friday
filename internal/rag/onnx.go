package rag

import (
	"fmt"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// ONNXRuntime wraps the ONNX runtime for embedding inference.
type ONNXRuntime struct {
	modelPath    string
	embeddingDim int
	maxSeqLen    int
	mu           sync.Mutex
	initialized  bool
}

// NewONNXRuntime creates a new ONNX runtime instance.
func NewONNXRuntime(modelPath string, embeddingDim, maxSeqLen int) (*ONNXRuntime, error) {
	// Initialize ONNX Runtime library (only once globally)
	ort.SetSharedLibraryPath(getONNXLibPath())
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to initialize ONNX Runtime: %w", err)
	}

	return &ONNXRuntime{
		modelPath:    modelPath,
		embeddingDim: embeddingDim,
		maxSeqLen:    maxSeqLen,
		initialized:  true,
	}, nil
}

// getONNXLibPath returns the ONNX Runtime library path from environment.
func getONNXLibPath() string {
	// This will be set via ONNXRUNTIME_LIB_PATH environment variable
	// The library handles this automatically
	return ""
}

// Embed runs inference on a batch of tokenized inputs and returns embeddings.
func (o *ONNXRuntime) Embed(inputIDs, attentionMask [][]int64) ([][]float32, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.initialized {
		return nil, fmt.Errorf("ONNX runtime not initialized")
	}

	batchSize := len(inputIDs)
	if batchSize == 0 {
		return nil, fmt.Errorf("empty input batch")
	}

	seqLen := len(inputIDs[0])

	// Flatten input data for tensor creation
	flatInputIDs := make([]int64, batchSize*seqLen)
	flatAttentionMask := make([]int64, batchSize*seqLen)

	for i := 0; i < batchSize; i++ {
		if len(inputIDs[i]) != seqLen || len(attentionMask[i]) != seqLen {
			return nil, fmt.Errorf("inconsistent sequence length in batch")
		}
		copy(flatInputIDs[i*seqLen:(i+1)*seqLen], inputIDs[i])
		copy(flatAttentionMask[i*seqLen:(i+1)*seqLen], attentionMask[i])
	}

	// Create input tensors
	inputIDsShape := ort.NewShape(int64(batchSize), int64(seqLen))
	inputIDsTensor, err := ort.NewTensor(inputIDsShape, flatInputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskShape := ort.NewShape(int64(batchSize), int64(seqLen))
	attentionMaskTensor, err := ort.NewTensor(attentionMaskShape, flatAttentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer attentionMaskTensor.Destroy()

	// Create output tensor
	// MiniLM output shape: [batch, seq_len, hidden_dim]
	outputShape := ort.NewShape(int64(batchSize), int64(seqLen), int64(o.embeddingDim))
	outputData := make([]float32, batchSize*seqLen*o.embeddingDim)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// Create session with tensors
	session, err := ort.NewAdvancedSession(
		o.modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"last_hidden_state"},
		[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor},
		[]ort.ArbitraryTensor{outputTensor},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}
	defer session.Destroy()

	// Run inference
	if err := session.Run(); err != nil {
		return nil, fmt.Errorf("ONNX inference failed: %w", err)
	}

	// Get output data from tensor
	rawOutput := outputTensor.GetData()

	// Apply mean pooling with attention mask
	embeddings := o.meanPooling(rawOutput, flatAttentionMask, batchSize, seqLen)

	return embeddings, nil
}

// meanPooling applies mean pooling over token embeddings using attention mask.
func (o *ONNXRuntime) meanPooling(
	tokenEmbeddings []float32,
	attentionMask []int64,
	batchSize, seqLen int,
) [][]float32 {
	embeddings := make([][]float32, batchSize)

	for b := 0; b < batchSize; b++ {
		embedding := make([]float32, o.embeddingDim)
		maskSum := float32(0)

		// Sum embeddings weighted by attention mask
		for t := 0; t < seqLen; t++ {
			maskVal := float32(attentionMask[b*seqLen+t])
			maskSum += maskVal

			if maskVal > 0 {
				for d := 0; d < o.embeddingDim; d++ {
					idx := b*seqLen*o.embeddingDim + t*o.embeddingDim + d
					embedding[d] += tokenEmbeddings[idx] * maskVal
				}
			}
		}

		// Divide by sum to get mean (avoid division by zero)
		if maskSum > 0 {
			for d := 0; d < o.embeddingDim; d++ {
				embedding[d] /= maskSum
			}
		}

		// L2 normalize the embedding
		embedding = l2Normalize(embedding)

		embeddings[b] = embedding
	}

	return embeddings
}

// l2Normalize normalizes a vector to unit length.
func l2Normalize(vec []float32) []float32 {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}

	norm := math.Sqrt(sumSq)
	if norm < 1e-12 {
		return vec
	}

	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(float64(v) / norm)
	}

	return result
}

// Close releases ONNX runtime resources.
func (o *ONNXRuntime) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.initialized = false

	// Note: DestroyEnvironment is global and should only be called
	// when the entire application is shutting down
	return nil
}

// EmbeddingDim returns the embedding dimension.
func (o *ONNXRuntime) EmbeddingDim() int {
	return o.embeddingDim
}

// IsInitialized returns whether the runtime is ready.
func (o *ONNXRuntime) IsInitialized() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.initialized
}
