package rag

import (
	"context"
	"fmt"

	"github.com/ashutoshrp06/telemetry-debugger/internal/config"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"go.uber.org/zap"
)

// Pipeline orchestrates the RAG retrieval process.
type Pipeline struct {
	retriever     *Retriever
	topK          int
	minSimilarity float32
	logger        *zap.Logger
}

// NewPipeline creates a new RAG pipeline from configuration.
func NewPipeline(cfg *config.Config, logger *zap.Logger) (*Pipeline, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	retrieverCfg := RetrieverConfig{
		QdrantHost:     cfg.Qdrant.Host,
		QdrantPort:     cfg.Qdrant.Port,
		CollectionName: cfg.Qdrant.Collection,
		EmbeddingConfig: EmbeddingConfig{
			ModelPath:    cfg.ONNX.ModelPath,
			VocabPath:    cfg.ONNX.VocabPath,
			MaxSeqLen:    cfg.ONNX.MaxSequenceLength,
			EmbeddingDim: cfg.ONNX.EmbeddingDim,
			BatchSize:    32,
		},
	}

	retriever, err := NewRetriever(retrieverCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create retriever: %w", err)
	}

	return &Pipeline{
		retriever:     retriever,
		topK:          cfg.RAG.TopK,
		minSimilarity: cfg.RAG.MinSimilarity,
		logger:        logger,
	}, nil
}

// NewPipelineWithRetriever creates a pipeline with a custom retriever.
func NewPipelineWithRetriever(retriever *Retriever, topK int, minSimilarity float32, logger *zap.Logger) *Pipeline {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Pipeline{
		retriever:     retriever,
		topK:          topK,
		minSimilarity: minSimilarity,
		logger:        logger,
	}
}

// Retrieve performs retrieval for a query.
func (p *Pipeline) Retrieve(ctx context.Context, query string) ([]types.RetrievedChunk, error) {
	if query == "" {
		return nil, nil
	}

	chunks, err := p.retriever.Search(ctx, query, p.topK, p.minSimilarity)
	if err != nil {
		p.logger.Error("Retrieval failed",
			zap.Error(err),
			zap.String("query_preview", truncateString(query, 50)))
		return nil, err
	}

	p.logger.Info("Retrieval completed",
		zap.Int("chunks_found", len(chunks)),
		zap.String("query_preview", truncateString(query, 50)))

	return chunks, nil
}

// RetrieveWithOptions allows customizing retrieval parameters per query.
func (p *Pipeline) RetrieveWithOptions(ctx context.Context, query string, topK int, minSimilarity float32) ([]types.RetrievedChunk, error) {
	if query == "" {
		return nil, nil
	}

	if topK <= 0 {
		topK = p.topK
	}
	if minSimilarity <= 0 {
		minSimilarity = p.minSimilarity
	}

	return p.retriever.Search(ctx, query, topK, minSimilarity)
}

// Close releases pipeline resources.
func (p *Pipeline) Close() error {
	if p.retriever != nil {
		return p.retriever.Close()
	}
	return nil
}

// CollectionName returns the Qdrant collection name.
func (p *Pipeline) CollectionName() string {
	return p.retriever.CollectionName()
}
