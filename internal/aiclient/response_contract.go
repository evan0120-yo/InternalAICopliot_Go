package aiclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type ExtractionStructuredResponse struct {
	Operation     string   `json:"operation"`
	Summary       string   `json:"summary"`
	StartAt       string   `json:"startAt"`
	EndAt         string   `json:"endAt"`
	Location      string   `json:"location"`
	MissingFields []string `json:"missingFields"`
}

func responseSchemaForContract(contract AnalyzeResponseContract) map[string]any {
	switch normalizeAnalyzeResponseContract(contract) {
	case AnalyzeResponseContractExtraction:
		return extractionResponseSchema()
	default:
		return consultResponseSchema()
	}
}

func parseAnalyzeBusinessResponse(raw []byte, contract AnalyzeResponseContract, code, message string) (infra.ConsultBusinessResponse, error) {
	switch normalizeAnalyzeResponseContract(contract) {
	case AnalyzeResponseContractExtraction:
		return parseExtractionResponseJSON(raw, code, message)
	default:
		return parseBusinessResponseJSON(raw, code, message)
	}
}

func extractionResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{"create", "update", "delete", "query"},
			},
			"summary":       map[string]any{"type": "string"},
			"startAt":       map[string]any{"type": "string"},
			"endAt":         map[string]any{"type": "string"},
			"location":      map[string]any{"type": "string"},
			"missingFields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"operation", "summary", "startAt", "endAt", "location", "missingFields"},
		"additionalProperties": false,
	}
}

func parseExtractionResponseJSON(raw []byte, code, message string) (infra.ConsultBusinessResponse, error) {
	result, err := ParseExtractionStructuredResponse(raw, code, message)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	body, err := json.Marshal(result)
	if err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError(code, message, http.StatusBadGateway)
	}

	return infra.ConsultBusinessResponse{
		Status:         true,
		StatusAns:      "LINE_TASK_EXTRACTED",
		Response:       string(body),
		ResponseDetail: "",
	}, nil
}

func isAllowedExtractionOperation(operation string) bool {
	switch operation {
	case "create", "update", "delete", "query":
		return true
	default:
		return false
	}
}

func ParseExtractionStructuredResponse(raw []byte, code, message string) (ExtractionStructuredResponse, error) {
	var result ExtractionStructuredResponse
	if err := json.Unmarshal(bytes.TrimSpace(raw), &result); err != nil {
		normalized := normalizeStructuredJSONObject(raw)
		if len(normalized) == 0 || json.Unmarshal(normalized, &result) != nil {
			return ExtractionStructuredResponse{}, infra.NewError(code, message, http.StatusBadGateway)
		}
	}

	result.Operation = strings.ToLower(strings.TrimSpace(result.Operation))
	result.Summary = strings.TrimSpace(result.Summary)
	result.StartAt = strings.TrimSpace(result.StartAt)
	result.EndAt = strings.TrimSpace(result.EndAt)
	result.Location = strings.TrimSpace(result.Location)
	result.MissingFields = normalizeMissingFields(result.MissingFields)

	if !isAllowedExtractionOperation(result.Operation) {
		return ExtractionStructuredResponse{}, infra.NewError(code, message, http.StatusBadGateway)
	}

	return result, nil
}

func normalizeMissingFields(fields []string) []string {
	if len(fields) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return []string{}
	}
	return normalized
}

func normalizeStructuredJSONObject(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}

	if stripped := stripMarkdownCodeFence(trimmed); len(stripped) > 0 {
		trimmed = stripped
	}
	if extracted, ok := extractFirstJSONObject(trimmed); ok {
		return extracted
	}
	return nil
}
