package promptguard

import (
	"context"
	"fmt"
	"strings"
)

// ServiceOption customizes promptguard service dependencies.
type ServiceOption func(*Service)

// WithScoreTextFunc overrides the first-layer text scoring function.
func WithScoreTextFunc(fn func(userText string) (Evaluation, error)) ServiceOption {
	return func(service *Service) {
		if fn != nil {
			service.scoreTextFn = fn
		}
	}
}

// WithGuardPromptAssembler wires the builder-owned guard prompt assembly step.
func WithGuardPromptAssembler(fn func(context.Context, Command) (GuardPrompt, error)) ServiceOption {
	return func(service *Service) {
		service.guardPromptFn = fn
	}
}

// WithCloudLLMFunc wires the hosted Gemma guard route.
func WithCloudLLMFunc(fn func(context.Context, GuardLLMRequest) (GuardLLMResponse, error)) ServiceOption {
	return func(service *Service) {
		service.cloudLLMFn = fn
	}
}

// WithLocalLLMFunc wires the local Gemma guard route.
func WithLocalLLMFunc(fn func(context.Context, GuardLLMRequest) (GuardLLMResponse, error)) ServiceOption {
	return func(service *Service) {
		service.localLLMFn = fn
	}
}

// Service executes the promptguard decision flow.
type Service struct {
	config Config

	scoreTextFn   func(userText string) (Evaluation, error)
	guardPromptFn func(context.Context, Command) (GuardPrompt, error)
	cloudLLMFn    func(context.Context, GuardLLMRequest) (GuardLLMResponse, error)
	localLLMFn    func(context.Context, GuardLLMRequest) (GuardLLMResponse, error)
}

// NewService constructs the promptguard service.
func NewService(config Config, options ...ServiceOption) *Service {
	service := &Service{config: config}
	service.scoreTextFn = service.scoreTextDefault
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// Evaluate runs text scoring first, then falls back to LLM guard when needed.
func (s *Service) Evaluate(ctx context.Context, command Command) (Evaluation, error) {
	evaluation, err := s.ScoreText(command.UserText)
	if err != nil {
		return Evaluation{}, err
	}

	switch evaluation.Decision {
	case DecisionAllow, DecisionBlock:
		return evaluation, nil
	case DecisionNeedsLLM:
		return s.EvaluateWithLLM(ctx, command)
	default:
		return Evaluation{}, fmt.Errorf("promptguard: unsupported decision %q", evaluation.Decision)
	}
}

// ScoreText is the first-layer text scoring entrypoint.
func (s *Service) ScoreText(userText string) (Evaluation, error) {
	return s.scoreTextFn(userText)
}

// EvaluateWithLLM routes second-layer guard evaluation to cloud or local mode.
func (s *Service) EvaluateWithLLM(ctx context.Context, command Command) (Evaluation, error) {
	llmMode := s.config.resolvedMode()
	llmFn := s.cloudLLMFn
	placeholderReason := llmGuardCloudPlaceholderReason
	if llmMode == ModeLocal {
		llmFn = s.localLLMFn
		placeholderReason = llmGuardLocalPlaceholderReason
	}

	if s.guardPromptFn == nil || llmFn == nil {
		return Evaluation{
			Decision: DecisionNeedsLLM,
			Score:    needsLLMPlaceholderScore,
			Reason:   placeholderReason,
			Source:   SourceLLMGuard,
		}, nil
	}

	guardPrompt, err := s.guardPromptFn(ctx, command)
	if err != nil {
		return Evaluation{}, err
	}

	llmResponse, err := llmFn(ctx, GuardLLMRequest{
		Mode:            llmMode,
		Model:           s.config.resolvedModel(),
		BaseURL:         s.config.resolvedBaseURL(),
		APIKey:          strings.TrimSpace(s.config.APIKey),
		Instructions:    guardPrompt.Instructions,
		UserMessageText: guardPrompt.UserMessageText,
	})
	if err != nil {
		return Evaluation{}, err
	}

	return mapGuardLLMResponse(llmResponse), nil
}

func (s *Service) scoreTextDefault(userText string) (Evaluation, error) {
	analysis := TextAnalysis{
		RawText:        userText,
		NormalizedText: normalizeText(userText),
	}
	analysis.Matches = matchFeatures(analysis.NormalizedText, defaultRuleCatalog)
	analysis.Score, analysis.MatchedCategories = scoreAnalysis(analysis.Matches)
	analysis.Decision, analysis.Reason = routeDecision(analysis.Score, analysis.Matches, analysis.MatchedCategories)
	return analysis.Evaluation(), nil
}

func mapGuardLLMResponse(response GuardLLMResponse) Evaluation {
	reason := strings.TrimSpace(response.Reason)
	if reason == "" {
		reason = strings.TrimSpace(response.StatusAns)
	}
	if reason == "" {
		reason = "LLM_GUARD_EMPTY_REASON"
	}

	if response.Status {
		return Evaluation{
			Decision: DecisionAllow,
			Score:    needsLLMPlaceholderScore,
			Reason:   reason,
			Source:   SourceLLMGuard,
		}
	}

	return Evaluation{
		Decision: DecisionBlock,
		Score:    needsLLMPlaceholderScore,
		Reason:   reason,
		Source:   SourceLLMGuard,
	}
}
