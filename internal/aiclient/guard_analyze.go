package aiclient

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"com.citrus.internalaicopilot/internal/infra"
)

// GuardAnalyzeRoute selects which promptguard LLM endpoint should be used.
type GuardAnalyzeRoute string

const (
	GuardAnalyzeRouteCloud GuardAnalyzeRoute = "cloud"
	GuardAnalyzeRouteLocal GuardAnalyzeRoute = "local"
)

// GuardAnalyzeCommand is the dedicated promptguard analyze contract.
type GuardAnalyzeCommand struct {
	Route           GuardAnalyzeRoute
	Model           string
	BaseURL         string
	APIKey          string
	Instructions    string
	UserMessageText string
}

// GuardAnalyzeResult is the parsed promptguard JSON contract.
type GuardAnalyzeResult struct {
	Status    bool   `json:"status"`
	StatusAns string `json:"statusAns"`
	Reason    string `json:"reason"`
}

// AnalyzeGuard executes the dedicated promptguard LLM path.
func (u *AnalyzeUseCase) AnalyzeGuard(ctx context.Context, command GuardAnalyzeCommand) (GuardAnalyzeResult, error) {
	return u.service.AnalyzeGuard(ctx, command)
}

// AnalyzeGuard executes the dedicated promptguard LLM path.
func (s *AnalyzeService) AnalyzeGuard(ctx context.Context, command GuardAnalyzeCommand) (result GuardAnalyzeResult, err error) {
	route := command.resolvedRoute()
	model := command.resolvedModel()
	startedAt := time.Now()

	log.Printf("ai promptguard started route=%s model=%s", route, model)
	defer func() {
		durationMs := time.Since(startedAt).Milliseconds()
		if err != nil {
			log.Printf("ai promptguard failed route=%s model=%s duration_ms=%d err=%v", route, model, durationMs, err)
			return
		}
		log.Printf("ai promptguard completed route=%s model=%s status=%t duration_ms=%d", route, model, result.Status, durationMs)
	}()

	switch route {
	case GuardAnalyzeRouteLocal:
		return s.analyzePromptGuardGemma(ctx, command, false)
	default:
		return s.analyzePromptGuardGemma(ctx, command, true)
	}
}

func (s *AnalyzeService) analyzePromptGuardGemma(ctx context.Context, command GuardAnalyzeCommand, requireAPIKey bool) (GuardAnalyzeResult, error) {
	baseURL := command.resolvedBaseURL()
	if !requireAPIKey && baseURL == "" {
		return GuardAnalyzeResult{}, infra.NewError("PROMPTGUARD_LOCAL_BASE_URL_MISSING", "Promptguard local base URL is required for local mode.", http.StatusInternalServerError)
	}

	raw, err := executeGemmaJSONAnalyze(ctx, s.httpClient, gemmaJSONRequest{
		APIKey:            strings.TrimSpace(command.APIKey),
		BaseURL:           baseURL,
		Model:             command.resolvedModel(),
		SystemInstruction: command.Instructions,
		Parts: []map[string]any{
			{"text": strings.TrimSpace(command.UserMessageText)},
		},
		ResponseSchema:    promptGuardResponseSchema(),
		RequireAPIKey:     requireAPIKey,
		MissingAPIKeyCode: "PROMPTGUARD_GEMMA_API_KEY_MISSING",
		MissingAPIKeyMsg:  "Promptguard Gemma API key is required for cloud mode.",
		FailureCode:       "PROMPTGUARD_ANALYSIS_FAILED",
		FailureMsg:        "Promptguard Gemma analysis failed.",
		EmptyOutputCode:   "PROMPTGUARD_EMPTY_OUTPUT",
		EmptyOutputMsg:    "Promptguard Gemma returned no structured response.",
		LogPrefix:         "promptguard generateContent",
	})
	if err != nil {
		return GuardAnalyzeResult{}, err
	}

	return parseGuardResponseJSON(raw, "PROMPTGUARD_ANALYSIS_FAILED", "Promptguard response did not match the expected JSON contract.")
}

func promptGuardResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":    map[string]any{"type": "boolean"},
			"statusAns": map[string]any{"type": "string"},
			"reason":    map[string]any{"type": "string"},
		},
		"required":             []string{"status", "statusAns", "reason"},
		"additionalProperties": false,
	}
}

func parseGuardResponseJSON(raw []byte, code, message string) (GuardAnalyzeResult, error) {
	var result GuardAnalyzeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return GuardAnalyzeResult{}, infra.NewError(code, message, http.StatusBadGateway)
	}
	return result, nil
}

func (c GuardAnalyzeCommand) resolvedRoute() GuardAnalyzeRoute {
	switch GuardAnalyzeRoute(strings.ToLower(strings.TrimSpace(string(c.Route)))) {
	case GuardAnalyzeRouteLocal:
		return GuardAnalyzeRouteLocal
	default:
		return GuardAnalyzeRouteCloud
	}
}

func (c GuardAnalyzeCommand) resolvedModel() string {
	if value := strings.TrimSpace(c.Model); value != "" {
		return value
	}
	return infra.DefaultGemmaModel
}

func (c GuardAnalyzeCommand) resolvedBaseURL() string {
	if value := strings.TrimSpace(c.BaseURL); value != "" {
		return value
	}
	if c.resolvedRoute() == GuardAnalyzeRouteCloud {
		return infra.DefaultGemmaBaseURL
	}
	return ""
}
