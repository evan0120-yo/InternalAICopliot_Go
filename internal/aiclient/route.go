package aiclient

import (
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

// AIRouteCode selects how aiclient should interact with models.
type AIRouteCode string

const (
	AIRouteDirectGemma    AIRouteCode = "direct_gemma"
	AIRouteDirectGPT54    AIRouteCode = "direct_gpt54"
	AIRouteGemmaThenGPT54 AIRouteCode = "gemma_then_gpt54"
)

// AnalyzeResponseContract selects which structured schema the model must return.
type AnalyzeResponseContract string

const (
	AnalyzeResponseContractConsult    AnalyzeResponseContract = "consult"
	AnalyzeResponseContractExtraction AnalyzeResponseContract = "extraction"
)

// AnalyzeCommand is the builder-facing AI execution contract.
type AnalyzeCommand struct {
	Route             AIRouteCode
	ResponseContract  AnalyzeResponseContract
	UserText          string
	Instructions      string
	PromptBodyPreview string
	Attachments       []infra.Attachment
	Mode              infra.AIExecutionMode
}

// DefaultAIRouteForConfig keeps legacy deployment-wide provider settings as a fallback route.
func DefaultAIRouteForConfig(config infra.Config) AIRouteCode {
	switch config.ResolvedAIProvider() {
	case infra.AIProviderGemma:
		return AIRouteDirectGemma
	default:
		return AIRouteDirectGPT54
	}
}

func normalizeAIRouteCode(value AIRouteCode, fallback AIRouteCode) AIRouteCode {
	switch AIRouteCode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AIRouteDirectGemma:
		return AIRouteDirectGemma
	case AIRouteGemmaThenGPT54:
		return AIRouteGemmaThenGPT54
	case AIRouteDirectGPT54:
		return AIRouteDirectGPT54
	default:
		return fallback
	}
}

func normalizeAnalyzeResponseContract(value AnalyzeResponseContract) AnalyzeResponseContract {
	switch AnalyzeResponseContract(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AnalyzeResponseContractExtraction:
		return AnalyzeResponseContractExtraction
	default:
		return AnalyzeResponseContractConsult
	}
}
