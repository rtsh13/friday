package rag

import (
	"context"
	"fmt"

	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"
)

// Retriever handles document retrieval from Qdrant using ONNX embeddings.
type Retriever struct {
	client         *qdrant.Client
	collectionName string
	embedder       *EmbeddingClient
	logger         *zap.Logger
}

// RetrieverConfig holds configuration for the retriever.
type RetrieverConfig struct {
	QdrantHost      string
	QdrantPort      int
	CollectionName  string
	EmbeddingConfig EmbeddingConfig
}

// NewRetriever creates a new retriever with ONNX embeddings.
func NewRetriever(cfg RetrieverConfig, logger *zap.Logger) (*Retriever, error) {
	// Connect to Qdrant
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.QdrantHost,
		Port: cfg.QdrantPort,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant at %s:%d: %w",
			cfg.QdrantHost, cfg.QdrantPort, err)
	}

	// Initialize ONNX embedder
	embedder, err := NewEmbeddingClient(cfg.EmbeddingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding client: %w", err)
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Retriever{
		client:         client,
		collectionName: cfg.CollectionName,
		embedder:       embedder,
		logger:         logger,
	}, nil
}

// Search performs semantic search on the Qdrant collection.
func (r *Retriever) Search(ctx context.Context, query string, topK int, minScore float32) ([]types.RetrievedChunk, error) {
	// Generate query embedding using ONNX
	queryEmbedding, err := r.embedder.EmbedSingle(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Convert topK to uint64 pointer
	limit := uint64(topK)

	// Search Qdrant
	searchResult, err := r.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: r.collectionName,
		Query:          qdrant.NewQuery(queryEmbedding...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("Qdrant search failed: %w", err)
	}

	// Convert results to RetrievedChunk
	chunks := make([]types.RetrievedChunk, 0, len(searchResult))
	for _, result := range searchResult {
		chunk := types.RetrievedChunk{
			Score:    float64(result.Score),
			Metadata: make(map[string]interface{}),
		}

		// Extract payload fields
		if result.Payload != nil {
			chunk.Metadata = convertPayload(result.Payload)

			if content, ok := getPayloadString(result.Payload, "content"); ok {
				chunk.Content = content
			}
			if source, ok := getPayloadString(result.Payload, "source"); ok {
				chunk.Source = source
			}
			if category, ok := getPayloadString(result.Payload, "category"); ok {
				chunk.Category = category
			}
		}

		chunks = append(chunks, chunk)
	}

	r.logger.Info("Search completed",
		zap.Int("results", len(chunks)),
		zap.String("query_preview", truncateString(query, 50)),
		zap.Float32("min_score", minScore))

	return chunks, nil
}

// Close releases retriever resources.
func (r *Retriever) Close() error {
	if r.embedder != nil {
		return r.embedder.Close()
	}
	return nil
}

// CollectionName returns the configured collection name.
func (r *Retriever) CollectionName() string {
	return r.collectionName
}

// getPayloadString extracts a string value from Qdrant payload.
func getPayloadString(payload map[string]*qdrant.Value, key string) (string, bool) {
	if val, ok := payload[key]; ok {
		if strVal := val.GetStringValue(); strVal != "" {
			return strVal, true
		}
	}
	return "", false
}

// convertPayload converts Qdrant payload to a generic map.
func convertPayload(payload map[string]*qdrant.Value) map[string]interface{} {
	result := make(map[string]interface{})
	for key, val := range payload {
		if val == nil {
			continue
		}
		switch v := val.Kind.(type) {
		case *qdrant.Value_StringValue:
			result[key] = v.StringValue
		case *qdrant.Value_IntegerValue:
			result[key] = v.IntegerValue
		case *qdrant.Value_DoubleValue:
			result[key] = v.DoubleValue
		case *qdrant.Value_BoolValue:
			result[key] = v.BoolValue
		case *qdrant.Value_ListValue:
			if v.ListValue != nil {
				list := make([]interface{}, 0, len(v.ListValue.Values))
				for _, item := range v.ListValue.Values {
					if item.GetStringValue() != "" {
						list = append(list, item.GetStringValue())
					}
				}
				result[key] = list
			}
		}
	}
	return result
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
