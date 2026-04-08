package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type openAIProvider struct {
	config infra.Config
	client *http.Client
}

func newOpenAIProvider(config infra.Config, client *http.Client) *openAIProvider {
	return &openAIProvider{
		config: config,
		client: client,
	}
}

func (p *openAIProvider) Analyze(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	if strings.TrimSpace(p.config.OpenAIAPIKey) == "" {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_API_KEY_MISSING", "OpenAI API key is required for live OpenAI mode.", http.StatusInternalServerError)
	}

	request.Model = requestModelOrFallback(request.Model, p.config.OpenAIModel)
	content, err := p.buildInputContent(ctx, request)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	payload := buildResponsesPayload(request, content)

	body, err := json.Marshal(payload)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.config.OpenAIBaseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.config.OpenAIAPIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_ANALYSIS_FAILED", "OpenAI consult analysis failed.", http.StatusBadGateway)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_ANALYSIS_FAILED", "Failed to read OpenAI response.", http.StatusBadGateway)
	}
	if response.StatusCode >= 400 {
		log.Printf("openai responses api failed: status=%d body=%s", response.StatusCode, previewBody(responseBody))
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_ANALYSIS_FAILED", "OpenAI consult analysis failed.", http.StatusBadGateway)
	}

	var parsed struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_ANALYSIS_FAILED", "OpenAI returned an unreadable response.", http.StatusBadGateway)
	}
	if len(parsed.Output) == 0 || len(parsed.Output[0].Content) == 0 {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_EMPTY_OUTPUT", "OpenAI returned no structured response.", http.StatusBadGateway)
	}

	return parseBusinessResponseJSON([]byte(parsed.Output[0].Content[0].Text), "OPENAI_ANALYSIS_FAILED", "OpenAI response did not match the expected JSON contract.")
}

func (p *openAIProvider) buildInputContent(ctx context.Context, request analyzeRequest) ([]map[string]any, error) {
	content := make([]map[string]any, 0, 1+len(request.Attachments))
	content = append(content, map[string]any{
		"type": "input_text",
		"text": request.UserText,
	})

	for _, attachment := range request.Attachments {
		fileID, kind, err := p.uploadFile(ctx, attachment)
		if err != nil {
			return nil, err
		}
		if kind == "image" {
			content = append(content, map[string]any{
				"type":    "input_image",
				"file_id": fileID,
				"detail":  "auto",
			})
			continue
		}
		content = append(content, map[string]any{
			"type":    "input_file",
			"file_id": fileID,
		})
	}
	return content, nil
}

func buildResponsesPayload(request analyzeRequest, content []map[string]any) map[string]any {
	return map[string]any{
		"model":        request.Model,
		"instructions": request.Instructions,
		"store":        false,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "consult_response",
				"schema": consultResponseSchema(),
			},
		},
		"input": []map[string]any{
			{
				"role":    "user",
				"content": content,
			},
		},
	}
}

func (p *openAIProvider) uploadFile(ctx context.Context, attachment infra.Attachment) (string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("purpose", uploadPurpose(attachment.FileName)); err != nil {
		return "", "", err
	}
	part, err := writer.CreateFormFile("file", attachment.FileName)
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(attachment.Data); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.config.OpenAIBaseURL, "/")+"/files", &body)
	if err != nil {
		return "", "", err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.config.OpenAIAPIKey)
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return "", "", infra.NewError("ATTACHMENT_UPLOAD_FAILED", "Attachment could not be uploaded to OpenAI.", http.StatusBadGateway)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", "", infra.NewError("ATTACHMENT_UPLOAD_FAILED", "Attachment upload response could not be read.", http.StatusBadGateway)
	}
	if response.StatusCode >= 400 {
		log.Printf("openai attachment upload rejected: status=%d body=%s filename=%s", response.StatusCode, previewBody(responseBody), attachment.FileName)
		return "", "", infra.NewError("ATTACHMENT_UPLOAD_REJECTED", "OpenAI rejected the uploaded attachment.", http.StatusBadGateway)
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil || parsed.ID == "" {
		return "", "", infra.NewError("ATTACHMENT_UPLOAD_FAILED", "OpenAI attachment upload did not return a valid file id.", http.StatusBadGateway)
	}

	kind := "file"
	if isImageName(attachment.FileName) {
		kind = "image"
	}
	return parsed.ID, kind, nil
}

func uploadPurpose(fileName string) string {
	if isImageName(fileName) {
		return "vision"
	}
	return "user_data"
}
