package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"com.citrus.internalaicopilot/internal/infra"
)

var (
	rawUserTextPattern = regexp.MustCompile(`(?s)## \[RAW_USER_TEXT\]\s*(.*?)\s*## \[`)
	builderCodePattern = regexp.MustCompile(`builderCode=([A-Za-z0-9_-]+)`)
)

type analyzeRequest struct {
	Model             string
	UserText          string
	Instructions      string
	PromptBodyPreview string
	Attachments       []infra.Attachment
	Mode              infra.AIExecutionMode
}

// AnalyzeService handles AI execution and upload details.
type AnalyzeService struct {
	config infra.Config
	client *http.Client
}

// NewAnalyzeService constructs the AI client service.
func NewAnalyzeService(config infra.Config) *AnalyzeService {
	return &AnalyzeService{
		config: config,
		client: &http.Client{
			Timeout: config.OpenAITimeout,
		},
	}
}

// Analyze runs the configured AI strategy.
func (s *AnalyzeService) Analyze(ctx context.Context, model, text, instructions, promptBodyPreview string, attachments []infra.Attachment, mode infra.AIExecutionMode) (infra.ConsultBusinessResponse, error) {
	request := analyzeRequest{
		Model:             model,
		UserText:          text,
		Instructions:      instructions,
		PromptBodyPreview: promptBodyPreview,
		Attachments:       attachments,
		Mode:              mode,
	}
	switch s.resolveAnalyzeMode(request.Mode) {
	case infra.AIExecutionModePreviewFull:
		return s.previewAnalyze(request)
	case infra.AIExecutionModePreviewPromptBodyOnly:
		return s.previewPromptBodyAnalyze(request)
	}
	if strings.TrimSpace(s.config.OpenAIAPIKey) == "" {
		return s.mockAnalyze(request), nil
	}
	return s.openAIAnalyze(ctx, request)
}

func (s *AnalyzeService) resolveAnalyzeMode(requestMode infra.AIExecutionMode) infra.AIExecutionMode {
	switch requestMode {
	case infra.AIExecutionModePreviewFull, infra.AIExecutionModePreviewPromptBodyOnly, infra.AIExecutionModeLive:
		return requestMode
	default:
		if s.config.AIDefaultMode != "" {
			return s.config.AIDefaultMode
		}
		if s.config.AIPreviewMode {
			return infra.AIExecutionModePreviewFull
		}
		return infra.AIExecutionModeLive
	}
}

func (s *AnalyzeService) previewAnalyze(request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	return infra.ConsultBusinessResponse{
		Status:    true,
		StatusAns: "PROMPT_PREVIEW",
		Response:  buildPreviewText(request),
		Preview:   true,
	}, nil
}

func (s *AnalyzeService) previewPromptBodyAnalyze(request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	return infra.ConsultBusinessResponse{
		Status:    true,
		StatusAns: "PROMPT_PREVIEW",
		Response:  strings.TrimSpace(request.PromptBodyPreview),
		Preview:   true,
	}, nil
}

func (s *AnalyzeService) mockAnalyze(request analyzeRequest) infra.ConsultBusinessResponse {
	rawUserText := extractRawUserText(request.Instructions)
	if looksLikePromptInjection(rawUserText) {
		return infra.ConsultBusinessResponse{
			Status:    false,
			StatusAns: "prompts有違法注入內容",
			Response:  "取消回應",
		}
	}

	builderCode := extractBuilderCode(request.Instructions)
	switch builderCode {
	case "qa-smoke-doc":
		return infra.ConsultBusinessResponse{
			Status:    true,
			StatusAns: "",
			Response: `冒煙測試摘要
- 覆蓋首頁入口與 Rewards 主流程
- 補充重複點擊與跨頁返回場景

| 用例編號 | 需求 | 功能域 | 模塊細分 | 二級模塊細分 | 用例名稱 | 測試類型 | 前提條件 | 操作步驟 | 期望結果 | 用例級別 | 研发自测结果 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| TC-001 | Internal AI Copilot | APP | 首頁 | 浮動按鈕 | 點擊首頁浮動按鈕入口 | 功能測試 | 已登入 | 1、點擊首頁浮動按鈕入口 | 1、跳轉 Rewards 積分牆頁面 | S |  |
| TC-002 | Internal AI Copilot | APP | 積分牆 | 簽到區 | 重複點擊簽到按鈕 | 功能測試 | 已登入 | 1、點擊按鈕多次簽到 | 1、僅觸發一次領取，coin 不會重複發送 | S |  |`,
		}
	default:
		response := "需求理解\n- 先整理目標與邊界\n\n功能拆解與工時\n- 需求分析：0.5 人日\n- 開發與聯調：2 人日\n- 測試與修正：1 人日\n\n風險與待確認事項\n- 需再確認外部 API 契約與資料異常情境。"
		if len(request.Attachments) > 0 {
			response += fmt.Sprintf("\n\n附件補充\n- 本次共收到 %d 個附件，已納入分析上下文。", len(request.Attachments))
		}
		return infra.ConsultBusinessResponse{
			Status:    true,
			StatusAns: "",
			Response:  response,
		}
	}
}

func (s *AnalyzeService) openAIAnalyze(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	content, err := s.buildOpenAIInputContent(ctx, request)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	payload := buildResponsesPayload(request, content)

	body, err := json.Marshal(payload)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.OpenAIBaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.config.OpenAIAPIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(httpRequest)
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

	var businessResponse infra.ConsultBusinessResponse
	if err := json.Unmarshal([]byte(parsed.Output[0].Content[0].Text), &businessResponse); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("OPENAI_ANALYSIS_FAILED", "OpenAI response did not match the expected JSON contract.", http.StatusBadGateway)
	}
	return businessResponse, nil
}

func (s *AnalyzeService) buildOpenAIInputContent(ctx context.Context, request analyzeRequest) ([]map[string]any, error) {
	content := make([]map[string]any, 0, 1+len(request.Attachments))
	content = append(content, map[string]any{
		"type": "input_text",
		"text": request.UserText,
	})

	for _, attachment := range request.Attachments {
		fileID, kind, err := s.uploadFile(ctx, attachment)
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
				"type": "json_schema",
				"name": "consult_response",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status":         map[string]any{"type": "boolean"},
						"statusAns":      map[string]any{"type": "string"},
						"response":       map[string]any{"type": "string"},
						"responseDetail": map[string]any{"type": "string"},
					},
					"required":             []string{"status", "statusAns", "response", "responseDetail"},
					"additionalProperties": false,
				},
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

func buildPreviewText(request analyzeRequest) string {
	var builder strings.Builder
	builder.WriteString("## [INSTRUCTIONS]\n")
	builder.WriteString(strings.TrimSpace(request.Instructions))
	builder.WriteString("\n\n## [USER_MESSAGE]\n")
	builder.WriteString(strings.TrimSpace(request.UserText))

	if len(request.Attachments) > 0 {
		builder.WriteString("\n\n## [ATTACHMENTS]\n")
		for _, attachment := range request.Attachments {
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(attachment.FileName))
			builder.WriteString(" | ")
			builder.WriteString(strings.TrimSpace(attachment.ContentType))
			builder.WriteString(" | ")
			builder.WriteString(fmt.Sprintf("%d bytes", len(attachment.Data)))
			if isImageName(attachment.FileName) {
				builder.WriteString(" | image")
			} else {
				builder.WriteString(" | file")
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func (s *AnalyzeService) uploadFile(ctx context.Context, attachment infra.Attachment) (string, string, error) {
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

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.OpenAIBaseURL+"/files", &body)
	if err != nil {
		return "", "", err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.config.OpenAIAPIKey)
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := s.client.Do(httpRequest)
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

func isImageName(fileName string) bool {
	lower := strings.ToLower(fileName)
	for _, suffix := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func extractRawUserText(instructions string) string {
	match := rawUserTextPattern.FindStringSubmatch(instructions)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractBuilderCode(instructions string) string {
	match := builderCodePattern.FindStringSubmatch(instructions)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func looksLikePromptInjection(text string) bool {
	lower := strings.ToLower(text)
	for _, keyword := range []string{
		"ignore previous",
		"ignore all previous",
		"system prompt",
		"override instruction",
		"forget the rules",
		"忽略前面",
		"覆寫規則",
		"越權",
	} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func previewBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	runes := []rune(trimmed)
	if len(runes) <= 512 {
		return trimmed
	}
	preview := string(runes[:512])
	if utf8.ValidString(preview) {
		return preview + "..."
	}
	return strings.ToValidUTF8(preview, "") + "..."
}
