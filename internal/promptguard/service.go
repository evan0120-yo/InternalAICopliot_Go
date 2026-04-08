package promptguard

import "fmt"

// Service executes the promptguard decision flow.
type Service struct {
	config Config

	scoreTextFn func(rawUserText string) (Evaluation, error)
	cloudLLMFn  func(rawUserText string) (Evaluation, error)
	localLLMFn  func(rawUserText string) (Evaluation, error)
}

// NewService constructs the promptguard service.
func NewService(config Config) *Service {
	service := &Service{config: config}
	service.scoreTextFn = service.scoreTextDefault
	service.cloudLLMFn = service.evaluateWithCloudGemmaDefault
	service.localLLMFn = service.evaluateWithLocalGemmaDefault
	return service
}

// Evaluate runs text scoring first, then falls back to LLM guard when needed.
func (s *Service) Evaluate(rawUserText string) (Evaluation, error) {
	evaluation, err := s.ScoreText(rawUserText)
	if err != nil {
		return Evaluation{}, err
	}

	switch evaluation.Decision {
	case DecisionAllow, DecisionBlock:
		return evaluation, nil
	case DecisionNeedsLLM:
		return s.EvaluateWithLLM(rawUserText)
	default:
		return Evaluation{}, fmt.Errorf("promptguard: unsupported decision %q", evaluation.Decision)
	}
}

// ScoreText is the first-layer text scoring entrypoint.
func (s *Service) ScoreText(rawUserText string) (Evaluation, error) {
	return s.scoreTextFn(rawUserText)
}

// EvaluateWithLLM routes second-layer guard evaluation to cloud or local mode.
func (s *Service) EvaluateWithLLM(rawUserText string) (Evaluation, error) {
	switch s.config.resolvedMode() {
	case ModeLocal:
		return s.localLLMFn(rawUserText)
	default:
		return s.cloudLLMFn(rawUserText)
	}
}

func (s *Service) scoreTextDefault(rawUserText string) (Evaluation, error) {
	_ = rawUserText
	return Evaluation{
		Decision: DecisionNeedsLLM,
		Score:    needsLLMPlaceholderScore,
		Reason:   textRulePlaceholderReason,
		Source:   SourceTextRule,
	}, nil
}

func (s *Service) evaluateWithCloudGemmaDefault(rawUserText string) (Evaluation, error) {
	_ = rawUserText
	return Evaluation{
		Decision: DecisionNeedsLLM,
		Score:    needsLLMPlaceholderScore,
		Reason:   llmGuardCloudPlaceholderReason,
		Source:   SourceLLMGuard,
	}, nil
}

func (s *Service) evaluateWithLocalGemmaDefault(rawUserText string) (Evaluation, error) {
	_ = rawUserText
	return Evaluation{
		Decision: DecisionNeedsLLM,
		Score:    needsLLMPlaceholderScore,
		Reason:   llmGuardLocalPlaceholderReason,
		Source:   SourceLLMGuard,
	}, nil
}
