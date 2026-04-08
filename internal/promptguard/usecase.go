package promptguard

import "context"

// EvaluateUseCase is the caller-facing promptguard entrypoint.
type EvaluateUseCase struct {
	service *Service
}

// NewEvaluateUseCase builds the promptguard entrypoint.
func NewEvaluateUseCase(service *Service) *EvaluateUseCase {
	return &EvaluateUseCase{service: service}
}

// Evaluate runs promptguard evaluation for one gatekeeper command.
func (u *EvaluateUseCase) Evaluate(ctx context.Context, command Command) (Evaluation, error) {
	return u.service.Evaluate(ctx, command)
}
