package aiclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type analyzeRouteExecutor interface {
	Execute(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error)
}

type directAnalyzeRouteExecutor struct {
	provider liveAnalyzeProvider
	model    string
}

func (e directAnalyzeRouteExecutor) Execute(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	if e.provider == nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("AI_PROVIDER_UNSUPPORTED", "Configured AI provider is not supported.", http.StatusInternalServerError)
	}

	stageRequest := request
	stageRequest.Model = requestModelOrFallback(stageRequest.Model, e.model)
	return e.provider.Analyze(ctx, stageRequest)
}

type gemmaThenGPT54AnalyzeRouteExecutor struct {
	gemmaProvider  liveAnalyzeProvider
	gemmaModel     string
	openAIProvider liveAnalyzeProvider
	openAIModel    string
}

func (e gemmaThenGPT54AnalyzeRouteExecutor) Execute(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	if e.gemmaProvider == nil || e.openAIProvider == nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("AI_ROUTE_EXECUTOR_UNSUPPORTED", "Configured AI route is not supported.", http.StatusInternalServerError)
	}

	stage1Request := request
	stage1Request.Model = requestModelOrFallback(stage1Request.Model, e.gemmaModel)
	stage1Result, err := e.gemmaProvider.Analyze(ctx, stage1Request)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	if !stage1Result.Status {
		return stage1Result, nil
	}

	stage2Request := request
	stage2Request.Model = requestModelOrFallback(stage2Request.Model, e.openAIModel)
	stage2Request.Instructions = strings.TrimSpace(request.Instructions + "\n\n" + buildStage1GemmaBlock(stage1Result))
	return e.openAIProvider.Analyze(ctx, stage2Request)
}

func (s *AnalyzeService) liveRouteExecutor(route AIRouteCode) (analyzeRouteExecutor, error) {
	switch normalizeAIRouteCode(route, DefaultAIRouteForConfig(s.config)) {
	case AIRouteDirectGemma:
		provider, err := s.providerByKey(infra.AIProviderGemma)
		if err != nil {
			return nil, err
		}
		return directAnalyzeRouteExecutor{
			provider: provider,
			model:    s.defaultGemmaModel(),
		}, nil
	case AIRouteGemmaThenGPT54:
		gemmaProvider, err := s.providerByKey(infra.AIProviderGemma)
		if err != nil {
			return nil, err
		}
		openAIProvider, err := s.providerByKey(infra.AIProviderOpenAI)
		if err != nil {
			return nil, err
		}
		return gemmaThenGPT54AnalyzeRouteExecutor{
			gemmaProvider:  gemmaProvider,
			gemmaModel:     s.defaultGemmaModel(),
			openAIProvider: openAIProvider,
			openAIModel:    s.defaultGPT54Model(),
		}, nil
	default:
		provider, err := s.providerByKey(infra.AIProviderOpenAI)
		if err != nil {
			return nil, err
		}
		return directAnalyzeRouteExecutor{
			provider: provider,
			model:    s.defaultGPT54Model(),
		}, nil
	}
}

func (s *AnalyzeService) providerByKey(providerKey infra.AIProvider) (liveAnalyzeProvider, error) {
	provider, ok := s.providers[providerKey]
	if ok && provider != nil {
		return provider, nil
	}
	return nil, infra.NewError("AI_PROVIDER_UNSUPPORTED", "Configured AI provider is not supported.", http.StatusInternalServerError)
}

func (s *AnalyzeService) defaultOpenAIModel() string {
	return requestModelOrFallback("", s.config.OpenAIModel)
}

func (s *AnalyzeService) defaultGPT54Model() string {
	return strings.TrimSpace(infra.DefaultOpenAIModel)
}

func (s *AnalyzeService) defaultGemmaModel() string {
	return requestModelOrFallback("", s.config.GemmaModel)
}

func (s *AnalyzeService) describeRouteModel(route AIRouteCode) string {
	switch normalizeAIRouteCode(route, DefaultAIRouteForConfig(s.config)) {
	case AIRouteDirectGemma:
		return s.defaultGemmaModel()
	case AIRouteGemmaThenGPT54:
		return fmt.Sprintf("%s->%s", s.defaultGemmaModel(), s.defaultGPT54Model())
	default:
		return s.defaultGPT54Model()
	}
}

func buildStage1GemmaBlock(result infra.ConsultBusinessResponse) string {
	return fmt.Sprintf(`## [STAGE1_GEMMA_RESULT]
status: %t
statusAns: %s
response: %s
responseDetail: %s`, result.Status, strings.TrimSpace(result.StatusAns), strings.TrimSpace(result.Response), strings.TrimSpace(result.ResponseDetail))
}
