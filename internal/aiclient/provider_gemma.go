package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type gemmaProvider struct {
	config infra.Config
	client *http.Client
}

type gemmaUploadedFile struct {
	URI      string
	MIMEType string
}

func newGemmaProvider(config infra.Config, client *http.Client) *gemmaProvider {
	return &gemmaProvider{
		config: config,
		client: client,
	}
}

func (p *gemmaProvider) Analyze(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	request.Model = requestModelOrFallback(request.Model, p.config.GemmaModel)
	parts, err := p.buildParts(ctx, request)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	raw, err := executeGemmaJSONAnalyze(ctx, p.client, gemmaJSONRequest{
		APIKey:            p.config.GemmaAPIKey,
		BaseURL:           p.config.GemmaBaseURL,
		Model:             request.Model,
		SystemInstruction: request.Instructions,
		Parts:             parts,
		ResponseSchema:    responseSchemaForContract(request.ResponseContract),
		RequireAPIKey:     true,
		MissingAPIKeyCode: "GEMMA_API_KEY_MISSING",
		MissingAPIKeyMsg:  "Gemma API key is required for live Gemma mode.",
		FailureCode:       "GEMMA_ANALYSIS_FAILED",
		FailureMsg:        "Gemma consult analysis failed.",
		EmptyOutputCode:   "GEMMA_EMPTY_OUTPUT",
		EmptyOutputMsg:    "Gemma returned no structured response.",
		LogPrefix:         "gemma generateContent",
	})
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	return parseAnalyzeBusinessResponse(raw, request.ResponseContract, "GEMMA_ANALYSIS_FAILED", "Gemma response did not match the expected JSON contract.")
}

func (p *gemmaProvider) buildParts(ctx context.Context, request analyzeRequest) ([]map[string]any, error) {
	parts := make([]map[string]any, 0, 1+len(request.Attachments))
	if strings.TrimSpace(request.UserText) != "" || len(request.Attachments) == 0 {
		parts = append(parts, map[string]any{
			"text": request.UserText,
		})
	}

	for _, attachment := range request.Attachments {
		uploaded, err := p.uploadFile(ctx, attachment)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{
			"file_data": map[string]any{
				"mime_type": uploaded.MIMEType,
				"file_uri":  uploaded.URI,
			},
		})
	}
	return parts, nil
}

func (p *gemmaProvider) uploadFile(ctx context.Context, attachment infra.Attachment) (gemmaUploadedFile, error) {
	mimeType := attachmentMimeType(attachment)
	metadataBody, err := json.Marshal(map[string]any{
		"file": map[string]any{
			"display_name": attachment.FileName,
		},
	})
	if err != nil {
		return gemmaUploadedFile{}, err
	}

	startRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveGemmaUploadURL(p.config.GemmaBaseURL), bytes.NewReader(metadataBody))
	if err != nil {
		return gemmaUploadedFile{}, err
	}
	startRequest.Header.Set("x-goog-api-key", p.config.GemmaAPIKey)
	startRequest.Header.Set("X-Goog-Upload-Protocol", "resumable")
	startRequest.Header.Set("X-Goog-Upload-Command", "start")
	startRequest.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.Itoa(len(attachment.Data)))
	startRequest.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)
	startRequest.Header.Set("Content-Type", "application/json")

	startResponse, err := p.client.Do(startRequest)
	if err != nil {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Attachment could not be uploaded to Gemma.", http.StatusBadGateway)
	}
	defer startResponse.Body.Close()

	startBody, err := io.ReadAll(startResponse.Body)
	if err != nil {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Gemma upload start response could not be read.", http.StatusBadGateway)
	}
	if startResponse.StatusCode >= 400 {
		log.Printf("gemma upload start failed: status=%d body=%s filename=%s", startResponse.StatusCode, previewBody(startBody), attachment.FileName)
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_REJECTED", "Gemma rejected the uploaded attachment.", http.StatusBadGateway)
	}

	uploadURL := strings.TrimSpace(startResponse.Header.Get("X-Goog-Upload-URL"))
	if uploadURL == "" {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Gemma upload start did not return an upload URL.", http.StatusBadGateway)
	}

	uploadRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(attachment.Data))
	if err != nil {
		return gemmaUploadedFile{}, err
	}
	uploadRequest.Header.Set("Content-Length", strconv.Itoa(len(attachment.Data)))
	uploadRequest.Header.Set("X-Goog-Upload-Offset", "0")
	uploadRequest.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	uploadRequest.Header.Set("Content-Type", mimeType)

	uploadResponse, err := p.client.Do(uploadRequest)
	if err != nil {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Attachment could not be uploaded to Gemma.", http.StatusBadGateway)
	}
	defer uploadResponse.Body.Close()

	uploadBody, err := io.ReadAll(uploadResponse.Body)
	if err != nil {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Gemma upload response could not be read.", http.StatusBadGateway)
	}
	if uploadResponse.StatusCode >= 400 {
		log.Printf("gemma upload finalize failed: status=%d body=%s filename=%s", uploadResponse.StatusCode, previewBody(uploadBody), attachment.FileName)
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_REJECTED", "Gemma rejected the uploaded attachment.", http.StatusBadGateway)
	}

	var parsed struct {
		File struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
		} `json:"file"`
	}
	if err := json.Unmarshal(uploadBody, &parsed); err != nil || strings.TrimSpace(parsed.File.URI) == "" {
		return gemmaUploadedFile{}, infra.NewError("GEMMA_ATTACHMENT_UPLOAD_FAILED", "Gemma attachment upload did not return a valid file uri.", http.StatusBadGateway)
	}

	resolvedMimeType := strings.TrimSpace(parsed.File.MIMEType)
	if resolvedMimeType == "" {
		resolvedMimeType = mimeType
	}
	return gemmaUploadedFile{
		URI:      parsed.File.URI,
		MIMEType: resolvedMimeType,
	}, nil
}

func resolveGemmaGenerateContentURL(baseURL, model string) string {
	return strings.TrimRight(baseURL, "/") + "/models/" + strings.TrimSpace(model) + ":generateContent"
}

func resolveGemmaUploadURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.Contains(trimmed, "/upload/") {
		return trimmed + "/files"
	}
	if strings.HasSuffix(trimmed, "/v1beta") {
		return strings.TrimSuffix(trimmed, "/v1beta") + "/upload/v1beta/files"
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return strings.TrimSuffix(trimmed, "/v1") + "/upload/v1/files"
	}
	return trimmed + "/upload/v1beta/files"
}
