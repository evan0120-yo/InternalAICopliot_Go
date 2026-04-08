package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type gemmaJSONRequest struct {
	APIKey            string
	BaseURL           string
	Model             string
	SystemInstruction string
	Parts             []map[string]any
	ResponseSchema    map[string]any
	RequireAPIKey     bool
	MissingAPIKeyCode string
	MissingAPIKeyMsg  string
	FailureCode       string
	FailureMsg        string
	EmptyOutputCode   string
	EmptyOutputMsg    string
	LogPrefix         string
}

func executeGemmaJSONAnalyze(ctx context.Context, client *http.Client, request gemmaJSONRequest) ([]byte, error) {
	if request.RequireAPIKey && strings.TrimSpace(request.APIKey) == "" {
		return nil, infra.NewError(request.MissingAPIKeyCode, request.MissingAPIKeyMsg, http.StatusInternalServerError)
	}

	payload := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]any{
				{"text": request.SystemInstruction},
			},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": request.Parts,
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType":   "application/json",
			"responseJsonSchema": request.ResponseSchema,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveGemmaGenerateContentURL(request.BaseURL, request.Model), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if apiKey := strings.TrimSpace(request.APIKey); apiKey != "" {
		httpRequest.Header.Set("x-goog-api-key", apiKey)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := client.Do(httpRequest)
	if err != nil {
		return nil, infra.NewError(request.FailureCode, request.FailureMsg, http.StatusBadGateway)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, infra.NewError(request.FailureCode, request.FailureMsg, http.StatusBadGateway)
	}
	if response.StatusCode >= 400 {
		log.Printf("%s failed: status=%d body=%s", request.LogPrefix, response.StatusCode, previewBody(responseBody))
		return nil, infra.NewError(request.FailureCode, request.FailureMsg, http.StatusBadGateway)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, infra.NewError(request.FailureCode, request.FailureMsg, http.StatusBadGateway)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, infra.NewError(request.EmptyOutputCode, request.EmptyOutputMsg, http.StatusBadGateway)
	}

	var textBuilder strings.Builder
	for _, part := range parsed.Candidates[0].Content.Parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		if textBuilder.Len() > 0 {
			textBuilder.WriteString("\n")
		}
		textBuilder.WriteString(part.Text)
	}
	if textBuilder.Len() == 0 {
		return nil, infra.NewError(request.EmptyOutputCode, request.EmptyOutputMsg, http.StatusBadGateway)
	}

	return []byte(textBuilder.String()), nil
}
