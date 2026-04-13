package aiclient

import (
	"context"

	"com.citrus.internalaicopilot/internal/infra"
)

// AnalyzeUseCase is the builder-facing AI entrypoint.
// It intentionally stays thin today so future multi-step AI orchestration has a stable boundary.
type AnalyzeUseCase struct {
	service *AnalyzeService
}

// NewAnalyzeUseCase builds the AI entrypoint.
func NewAnalyzeUseCase(service *AnalyzeService) *AnalyzeUseCase {
	return &AnalyzeUseCase{service: service}
}

// Analyze executes the configured AI flow.
func (u *AnalyzeUseCase) Analyze(ctx context.Context, command AnalyzeCommand) (infra.ConsultBusinessResponse, error) {
	return u.service.Analyze(ctx, command)
}
