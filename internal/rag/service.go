package rag

import (
	"context"

	"com.citrus.internalaicopilot/internal/infra"
)

// ResolveService resolves source-level rag configs into deterministic content.
type ResolveService struct {
	store *infra.Store
}

// NewResolveService builds a rag resolver service.
func NewResolveService(store *infra.Store) *ResolveService {
	return &ResolveService{store: store}
}

// ResolveBySourceID loads and normalizes rag configs for a source.
func (s *ResolveService) ResolveBySourceID(ctx context.Context, sourceID int64) ([]infra.RagSupplement, error) {
	rags, err := s.store.RagsBySourceIDContext(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	for index := range rags {
		if rags[index].RetrievalMode != "full_context" {
			rags[index].RetrievalMode = "full_context"
		}
	}
	return rags, nil
}
