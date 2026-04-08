package promptguard

// EvaluateUseCase is the caller-facing promptguard entrypoint.
type EvaluateUseCase struct {
	service *Service
}

// NewEvaluateUseCase builds the promptguard entrypoint.
func NewEvaluateUseCase(service *Service) *EvaluateUseCase {
	return &EvaluateUseCase{service: service}
}

// Evaluate runs promptguard evaluation for raw user text.
func (u *EvaluateUseCase) Evaluate(rawUserText string) (Evaluation, error) {
	return u.service.Evaluate(rawUserText)
}
