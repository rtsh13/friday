package rag

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stratos/cliche/internal/types"
	"go.uber.org/zap"
)

// convertPayload converts qdrant.Value map to interface{} map
// Uses getter methods to extract typed values from protobuf oneof
func convertPayload(payload map[string]*qdrant.Value) map[string]interface{} {
	result := make(map[string]interface{}, len(payload))
	for key, val := range payload {
		if val == nil {
			result[key] = nil
			continue
		}

		// Use getter methods to extract the actual value
		// Check Kind to determine which type is set
		kind := val.GetKind()
		if kind == nil {
			// No value set, store nil
			result[key] = nil
			continue
		}

		// Type switch on the Kind interface to extract the value
		switch v := kind.(type) {
		case *qdrant.Value_NullValue:
			result[key] = nil
		case *qdrant.Value_BoolValue:
			result[key] = v.BoolValue
		case *qdrant.Value_IntegerValue:
			result[key] = v.IntegerValue
		case *qdrant.Value_DoubleValue:
			result[key] = v.DoubleValue
		case *qdrant.Value_StringValue:
			result[key] = v.StringValue
		case *qdrant.Value_ListValue:
			result[key] = v.ListValue
		case *qdrant.Value_StructValue:
			result[key] = v.StructValue
		default:
			// Fallback: store the whole value object
			result[key] = val
		}
	}
	return result
}

type Retriever struct {
	client         *qdrant.Client
	collectionName string
	embedder       *EmbeddingClient
	logger         *zap.Logger
}

func NewRetriever(host string, port int, embedEndpoint string, logger *zap.Logger) (*Retriever, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: int(port),
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

// Search searches for chunks similar to the query
func (r *Retriever) Search(ctx context.Context, query string, topK int, minScore float32) ([]types.RetrievedChunk, error) {
	// Step 1: Embed the query
	embeddings, err := r.embedder.Embed([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// Step 2: Search in Qdrant
	searchResult, err := r.client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
		CollectionName: r.collectionName,
		Vector:         embeddings[0],
		Limit:          uint64(topK),
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Step 3: Convert Qdrant results to RetrievedChunk
	chunks := make([]types.RetrievedChunk, 0, len(searchResult.Result))
	for _, result := range searchResult.Result {
		// Convert qdrant.Value map to interface{} map
		metadata := convertPayload(result.Payload)

		chunk := types.RetrievedChunk{
			Score:    float64(result.Score),
			Metadata: metadata,
		}

		// Extract typed fields from payload
		if content, ok := metadata["content"].(string); ok {
			chunk.Content = content
		}
		if source, ok := metadata["source"].(string); ok {
			chunk.Source = source
		}
		if category, ok := metadata["category"].(string); ok {
			chunk.Category = category
		}

		chunks = append(chunks, chunk)
	}

	// Log completion with truncated query
	r.logger.Info("Search completed",
		zap.Int("results", len(chunks)),
		zap.String("query", truncateString(query, 50)),
	)

	return chunks, nil
}

// Helper function to truncate strings
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
