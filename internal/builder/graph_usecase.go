package builder

import "context"

// GraphUseCase is the public admin graph entrypoint.
// It intentionally stays thin today so admin orchestration can grow without changing handlers.
type GraphUseCase struct {
	service *GraphService
}

// NewGraphUseCase builds the graph entrypoint.
func NewGraphUseCase(service *GraphService) *GraphUseCase {
	return &GraphUseCase{service: service}
}

// LoadGraph returns the builder graph.
func (u *GraphUseCase) LoadGraph(ctx context.Context, builderID int) (BuilderGraphResponse, error) {
	return u.service.LoadGraph(ctx, builderID)
}

// SaveGraph persists the builder graph.
func (u *GraphUseCase) SaveGraph(ctx context.Context, builderID int, request BuilderGraphRequest) (BuilderGraphResponse, error) {
	return u.service.SaveGraph(ctx, builderID, request)
}
