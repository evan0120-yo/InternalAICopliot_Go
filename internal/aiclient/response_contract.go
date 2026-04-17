package aiclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type ExtractionStructuredResponse struct {
	TaskType      string   `json:"taskType"`
	Operation     string   `json:"operation"`
	EventID       string   `json:"eventId"`
	Summary       string   `json:"summary"`
	StartAt       string   `json:"startAt"`
	EndAt         string   `json:"endAt"`
	QueryStartAt  string   `json:"queryStartAt"`
	QueryEndAt    string   `json:"queryEndAt"`
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
			"taskType": map[string]any{
				"type": "string",
			},
			"operation": map[string]any{
				"type": "string",
				"enum": []string{"create", "update", "delete", "query"},
			},
			"eventId":       map[string]any{"type": "string"},
			"summary":       map[string]any{"type": "string"},
			"startAt":       map[string]any{"type": "string"},
			"endAt":         map[string]any{"type": "string"},
			"queryStartAt":  map[string]any{"type": "string"},
			"queryEndAt":    map[string]any{"type": "string"},
			"location":      map[string]any{"type": "string"},
			"missingFields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"taskType", "operation", "eventId", "summary", "startAt", "endAt", "queryStartAt", "queryEndAt", "location", "missingFields"},
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

	result.TaskType = strings.ToLower(strings.TrimSpace(result.TaskType))
	result.Operation = strings.ToLower(strings.TrimSpace(result.Operation))
	result.EventID = strings.TrimSpace(result.EventID)
	result.Summary = strings.TrimSpace(result.Summary)
	result.StartAt = strings.TrimSpace(result.StartAt)
	result.EndAt = strings.TrimSpace(result.EndAt)
	result.QueryStartAt = strings.TrimSpace(result.QueryStartAt)
	result.QueryEndAt = strings.TrimSpace(result.QueryEndAt)
	result.Location = strings.TrimSpace(result.Location)
	result.MissingFields = normalizeMissingFields(result.MissingFields)

	if result.TaskType == "" {
		return ExtractionStructuredResponse{}, infra.NewError(code, message, http.StatusBadGateway)
	}
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
