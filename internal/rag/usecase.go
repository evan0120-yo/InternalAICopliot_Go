package rag

import (
	"context"

	"com.citrus.internalaicopilot/internal/infra"
)

// ResolveUseCase is the public builder-facing rag entrypoint.
// It intentionally stays thin today so future retrieval orchestration has a stable boundary.
type ResolveUseCase struct {
	service *ResolveService
}

// NewResolveUseCase builds the rag entrypoint.
func NewResolveUseCase(service *ResolveService) *ResolveUseCase {
	return &ResolveUseCase{service: service}
}

// ResolveBySourceID loads source supplements.
func (u *ResolveUseCase) ResolveBySourceID(ctx context.Context, sourceID int64) ([]infra.RagSupplement, error) {
	return u.service.ResolveBySourceID(ctx, sourceID)
}
