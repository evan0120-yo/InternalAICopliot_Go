package aiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"com.citrus.internalaicopilot/internal/infra"
)

var (
	builderCodePattern = regexp.MustCompile(`builderCode=([A-Za-z0-9_-]+)`)
)

type analyzeRequest struct {
	Route             AIRouteCode
	Model             string
	ResponseContract  AnalyzeResponseContract
	UserText          string
	Instructions      string
	PromptBodyPreview string
	Attachments       []infra.Attachment
	Mode              infra.AIExecutionMode
}

type liveAnalyzeProvider interface {
	Analyze(ctx context.Context, request analyzeRequest) (infra.ConsultBusinessResponse, error)
}

// AnalyzeService handles AI execution mode and provider routing.
type AnalyzeService struct {
	config     infra.Config
	httpClient *http.Client
	providers  map[infra.AIProvider]liveAnalyzeProvider
}

// NewAnalyzeService constructs the AI client service.
func NewAnalyzeService(config infra.Config) *AnalyzeService {
	client := &http.Client{Timeout: config.OpenAITimeout}
	return newAnalyzeServiceWithProviders(config, map[infra.AIProvider]liveAnalyzeProvider{
		infra.AIProviderOpenAI: newOpenAIProvider(config, client),
		infra.AIProviderGemma:  newGemmaProvider(config, client),
	}, client)
}

func newAnalyzeServiceWithProviders(config infra.Config, providers map[infra.AIProvider]liveAnalyzeProvider, client *http.Client) *AnalyzeService {
	return &AnalyzeService{
		config:     config,
		httpClient: client,
		providers:  providers,
	}
}

// Analyze runs the configured AI strategy.
func (s *AnalyzeService) Analyze(ctx context.Context, command AnalyzeCommand) (response infra.ConsultBusinessResponse, err error) {
	request := analyzeRequest{
		Route:             normalizeAIRouteCode(command.Route, DefaultAIRouteForConfig(s.config)),
		ResponseContract:  normalizeAnalyzeResponseContract(command.ResponseContract),
		UserText:          command.UserText,
		Instructions:      command.Instructions,
		PromptBodyPreview: command.PromptBodyPreview,
		Attachments:       command.Attachments,
		Mode:              command.Mode,
	}
	resolvedMode := s.resolveAnalyzeMode(request.Mode)
	executionRoute := s.describeExecutionRoute(resolvedMode, request.Route)
	builderCode := extractBuilderCode(request.Instructions)
	modelDescription := s.describeRouteModel(request.Route)
	startedAt := time.Now()

	log.Printf("ai analyze started mode=%s route=%s model=%s attachments=%d builderCode=%s", resolvedMode, executionRoute, strings.TrimSpace(modelDescription), len(request.Attachments), builderCode)
	defer func() {
		durationMs := time.Since(startedAt).Milliseconds()
		if err != nil {
			log.Printf("ai analyze failed mode=%s route=%s model=%s attachments=%d builderCode=%s duration_ms=%d err=%v", resolvedMode, executionRoute, strings.TrimSpace(modelDescription), len(request.Attachments), builderCode, durationMs, err)
			return
		}
		log.Printf("ai analyze completed mode=%s route=%s model=%s attachments=%d builderCode=%s status=%t preview=%t duration_ms=%d", resolvedMode, executionRoute, strings.TrimSpace(modelDescription), len(request.Attachments), builderCode, response.Status, response.Preview, durationMs)
	}()

	switch resolvedMode {
	case infra.AIExecutionModePreviewFull:
		return s.previewAnalyze(request)
	case infra.AIExecutionModePreviewPromptBodyOnly:
		return s.previewPromptBodyAnalyze(request)
	}
	if s.config.AIMockMode {
		return s.mockAnalyze(request), nil
	}

	executor, err := s.liveRouteExecutor(request.Route)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	return executor.Execute(ctx, request)
}

func (s *AnalyzeService) describeExecutionRoute(mode infra.AIExecutionMode, route AIRouteCode) string {
	switch mode {
	case infra.AIExecutionModePreviewFull, infra.AIExecutionModePreviewPromptBodyOnly:
		return "preview"
	case infra.AIExecutionModeLive:
		if s.config.AIMockMode {
			return "mock"
		}
		return string(normalizeAIRouteCode(route, DefaultAIRouteForConfig(s.config)))
	default:
		return "unknown"
	}
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
		Status:         true,
		StatusAns:      "PROMPT_PREVIEW",
		Response:       buildPreviewText(request),
		ResponseDetail: "",
		Preview:        true,
	}, nil
}

func (s *AnalyzeService) previewPromptBodyAnalyze(request analyzeRequest) (infra.ConsultBusinessResponse, error) {
	return infra.ConsultBusinessResponse{
		Status:         true,
		StatusAns:      "PROMPT_PREVIEW",
		Response:       strings.TrimSpace(request.PromptBodyPreview),
		ResponseDetail: "",
		Preview:        true,
	}, nil
}

func (s *AnalyzeService) mockAnalyze(request analyzeRequest) infra.ConsultBusinessResponse {
	if request.ResponseContract == AnalyzeResponseContractExtraction {
		return infra.ConsultBusinessResponse{
			Status:         true,
			StatusAns:      "LINE_TASK_EXTRACTED",
			Response:       `{"taskType":"calendar","operation":"create","summary":"mock event","startAt":"2026-04-15 15:00:00","endAt":"2026-04-15 15:30:00","location":"","missingFields":[]}`,
			ResponseDetail: "mock extraction fallback",
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
			ResponseDetail: "mock analyze qa-smoke-doc template",
		}
	default:
		response := "需求理解\n- 先整理目標與邊界\n\n功能拆解與工時\n- 需求分析：0.5 人日\n- 開發與聯調：2 人日\n- 測試與修正：1 人日\n\n風險與待確認事項\n- 需再確認外部 API 契約與資料異常情境。"
		if len(request.Attachments) > 0 {
			response += fmt.Sprintf("\n\n附件補充\n- 本次共收到 %d 個附件，已納入分析上下文。", len(request.Attachments))
		}
		return infra.ConsultBusinessResponse{
			Status:         true,
			StatusAns:      "",
			Response:       response,
			ResponseDetail: "mock analyze general fallback",
		}
	}
}

func consultResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":         map[string]any{"type": "boolean"},
			"statusAns":      map[string]any{"type": "string"},
			"response":       map[string]any{"type": "string"},
			"responseDetail": map[string]any{"type": "string"},
		},
		"required":             []string{"status", "statusAns", "response", "responseDetail"},
		"additionalProperties": false,
	}
}

func parseBusinessResponseJSON(raw []byte, code, message string) (infra.ConsultBusinessResponse, error) {
	var businessResponse infra.ConsultBusinessResponse
	if err := json.Unmarshal(raw, &businessResponse); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError(code, message, http.StatusBadGateway)
	}
	return businessResponse, nil
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

func requestModelOrFallback(requestModel, fallback string) string {
	if value := strings.TrimSpace(requestModel); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func attachmentMimeType(attachment infra.Attachment) string {
	if value := strings.TrimSpace(attachment.ContentType); value != "" {
		return value
	}
	if ext := strings.TrimSpace(filepath.Ext(attachment.FileName)); ext != "" {
		if resolved := mime.TypeByExtension(ext); resolved != "" {
			return resolved
		}
	}
	return "application/octet-stream"
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

func extractBuilderCode(instructions string) string {
	match := builderCodePattern.FindStringSubmatch(instructions)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
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
