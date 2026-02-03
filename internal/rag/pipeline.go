package rag

import (
	"context"

	"github.com/stratos/cliche/internal/types"
	"go.uber.org/zap"
)

type Pipeline struct {
	retriever *Retriever
	logger    *zap.Logger
}

func NewPipeline(host string, port int, embedEndpoint string, logger *zap.Logger) (*Pipeline, error) {
	retriever, err := NewRetriever(host, port, embedEndpoint, logger)
	if err != nil {
		return nil, err
	}

	return &Pipeline{
		retriever: retriever,
		logger:    logger,
	}, nil
}

func (p *Pipeline) Retrieve(ctx context.Context, query string) ([]types.RetrievedChunk, error) {
	return p.retriever.Search(ctx, query, 5, 0.7)
}
