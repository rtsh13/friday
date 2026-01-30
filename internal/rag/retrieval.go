package rag

import (
	"context"
	"fmt"

	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"
)

type Retriever struct {
	client         qdrant.QdrantClient
	collectionName string
	embedder       *EmbeddingClient
	logger         *zap.Logger
}

func NewRetriever(host string, port int, embedEndpoint string, logger *zap.Logger) (*Retriever, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: uint16(port),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}
	
	embedder := NewEmbeddingClient(embedEndpoint)
	
	return &Retriever{
		client:         client,
		collectionName: "telemetry_docs",
		embedder:       embedder,
		logger:         logger,
	}, nil
}

func (r *Retriever) Search(ctx context.Context, query string, topK int, minScore float32) ([]types.RetrievedChunk, error) {
	embeddings, err := r.embedder.Embed([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	
	searchResult, err := r.client.Search(ctx, &qdrant.SearchPoints{
		CollectionName: r.collectionName,
		Vector:         embeddings[0],
		Limit:          uint64(topK),
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	
	chunks := make([]types.RetrievedChunk, 0, len(searchResult))
	for _, result := range searchResult {
		chunk := types.RetrievedChunk{
			Score:    float64(result.Score),
			Metadata: result.Payload,
		}
		
		if content, ok := result.Payload["content"].(string); ok {
			chunk.Content = content
		}
		if source, ok := result.Payload["source"].(string); ok {
			chunk.Source = source
		}
		if category, ok := result.Payload["category"].(string); ok {
			chunk.Category = category
		}
		
		chunks = append(chunks, chunk)
	}
	
	r.logger.Info("Search completed", 
		zap.Int("results", len(chunks)),
		zap.String("query", query[:min(50, len(query))]))
	
	return chunks, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}