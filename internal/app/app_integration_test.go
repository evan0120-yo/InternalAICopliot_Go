package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/gatekeeper"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"
)

func TestAppSupportsBuilderListAndConsultFlow(t *testing.T) {
	app, err := New(newIntegrationTestConfig(t))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	buildersResponse, err := http.Get(server.URL + "/api/builders")
	if err != nil {
		t.Fatalf("GET /api/builders returned error: %v", err)
	}
	defer buildersResponse.Body.Close()
	if buildersResponse.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/builders returned status %d", buildersResponse.StatusCode)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("builderId", "2"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("text", "請幫我產出 Rewards 冒煙測試"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/consult", &body)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /api/consult returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		bytes, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /api/consult returned status %d body=%s", response.StatusCode, string(bytes))
	}

	var envelope infra.APIResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	dataBytes, _ := json.Marshal(envelope.Data)
	var consultResponse infra.ConsultBusinessResponse
	if err := json.Unmarshal(dataBytes, &consultResponse); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if !consultResponse.Status {
		t.Fatalf("expected consult to succeed, got %+v", consultResponse)
	}
	if consultResponse.File == nil || !strings.HasSuffix(consultResponse.File.FileName, ".xlsx") {
		t.Fatalf("expected xlsx file payload, got %+v", consultResponse.File)
	}
}

func TestAppSupportsExternalBuilderListAndConsultFlow(t *testing.T) {
	app, err := New(newIntegrationTestConfig(t))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	buildersRequest, err := http.NewRequest(http.MethodGet, server.URL+"/api/external/builders", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	buildersRequest.Header.Set(gatekeeper.ExternalAppIDHeader, "linkchat")
	buildersResponse, err := http.DefaultClient.Do(buildersRequest)
	if err != nil {
		t.Fatalf("GET /api/external/builders returned error: %v", err)
	}
	defer buildersResponse.Body.Close()
	if buildersResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(buildersResponse.Body)
		t.Fatalf("GET /api/external/builders returned status %d body=%s", buildersResponse.StatusCode, string(body))
	}

	var buildersEnvelope infra.APIResponse
	if err := json.NewDecoder(buildersResponse.Body).Decode(&buildersEnvelope); err != nil {
		t.Fatalf("Decode builders returned error: %v", err)
	}
	buildersBytes, _ := json.Marshal(buildersEnvelope.Data)
	var builders []builder.BuilderSummary
	if err := json.Unmarshal(buildersBytes, &builders); err != nil {
		t.Fatalf("Unmarshal builders returned error: %v", err)
	}
	if len(builders) != 3 {
		t.Fatalf("expected 3 builders for linkchat, got %+v", builders)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("builderId", "2"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("text", "請幫我產出 Rewards 冒煙測試"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/external/consult", &body)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set(gatekeeper.ExternalAppIDHeader, "linkchat")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /api/external/consult returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		bytes, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /api/external/consult returned status %d body=%s", response.StatusCode, string(bytes))
	}

	var envelope infra.APIResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("Decode consult returned error: %v", err)
	}
	dataBytes, _ := json.Marshal(envelope.Data)
	var consultResponse infra.ConsultBusinessResponse
	if err := json.Unmarshal(dataBytes, &consultResponse); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !consultResponse.Status {
		t.Fatalf("expected external consult to succeed, got %+v", consultResponse)
	}
}

func TestAppSupportsLineTaskConsultFlow(t *testing.T) {
	app := newLineTaskIntegrationApp(t)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody := strings.NewReader(`{
		"builderId":4,
		"messageText":"小傑 明天 下午三點找我吃飯",
		"referenceTime":"2026-04-14 10:00:00",
		"timeZone":"Asia/Taipei"
	}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/line-task-consult", requestBody)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /api/line-task-consult returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /api/line-task-consult returned status %d body=%s", response.StatusCode, string(body))
	}

	var envelope infra.APIResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope, got %+v", envelope)
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var lineTaskResponse struct {
		Operation string `json:"operation"`
		Summary   string `json:"summary"`
		StartAt   string `json:"startAt"`
		EndAt     string `json:"endAt"`
	}
	if err := json.Unmarshal(dataBytes, &lineTaskResponse); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if lineTaskResponse.Operation != "create" || lineTaskResponse.StartAt == "" || lineTaskResponse.EndAt == "" {
		t.Fatalf("unexpected line task response: %+v", lineTaskResponse)
	}
}

func TestAppSupportsCORSPreflightAndTemplateCreateStatus(t *testing.T) {
	app, err := New(newIntegrationTestConfig(t))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	optionsRequest, err := http.NewRequest(http.MethodOptions, server.URL+"/api/builders", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	optionsRequest.Header.Set("Origin", "http://localhost:3000")
	optionsResponse, err := http.DefaultClient.Do(optionsRequest)
	if err != nil {
		t.Fatalf("OPTIONS /api/builders returned error: %v", err)
	}
	defer optionsResponse.Body.Close()
	if optionsResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", optionsResponse.StatusCode)
	}
	if optionsResponse.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected CORS header, got %q", optionsResponse.Header.Get("Access-Control-Allow-Origin"))
	}

	requestBody := strings.NewReader(`{"templateKey":"qa-created","name":"QA Created","groupKey":"qa","orderNo":1,"prompts":"prompt"}`)
	createResponse, err := http.Post(server.URL+"/api/admin/templates", "application/json", requestBody)
	if err != nil {
		t.Fatalf("POST /api/admin/templates returned error: %v", err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResponse.Body)
		t.Fatalf("expected 201 for create template, got %d body=%s", createResponse.StatusCode, string(body))
	}
}

func newIntegrationTestConfig(t *testing.T) infra.Config {
	t.Helper()
	return infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
		FirestoreProjectID:  fmt.Sprintf("internal-ai-copilot-app-test-%d", time.Now().UnixNano()),
		StoreResetOnStart:   true,
		AIMockMode:          true,
	}
}

func newLineTaskIntegrationSeed() infra.StoreSeedData {
	seed := infra.DefaultSeedData()
	seed.Builders = append(seed.Builders, infra.BuilderConfig{
		BuilderID:   4,
		BuilderCode: "line-memo-crud",
		GroupLabel:  "LineBot",
		Name:        "Line 備忘錄抽取",
		Description: "供 app integration 驗證 line task extraction 路徑。",
		IncludeFile: false,
		FilePrefix:  "line-memo-crud",
		Active:      true,
	})
	seed.Sources = append(seed.Sources, infra.Source{
		SourceID:           1001,
		BuilderID:          4,
		Prompts:            "你現在負責將 LINE 口語訊息轉成固定 extraction JSON。",
		OrderNo:            1,
		SystemBlock:        false,
		NeedsRagSupplement: false,
	})
	return seed
}

func newLineTaskIntegrationApp(t *testing.T) *App {
	t.Helper()

	cfg := newIntegrationTestConfig(t)
	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:     cfg.FirestoreProjectID,
		EmulatorHost:  cfg.FirestoreEmulatorHost,
		SeedWhenEmpty: true,
		ResetOnStart:  cfg.StoreResetOnStart,
		SeedData:      newLineTaskIntegrationSeed(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ragUseCase := rag.NewResolveUseCase(rag.NewResolveService(store))
	aiUseCase := aiclient.NewAnalyzeUseCase(aiclient.NewAnalyzeService(cfg))
	outputUseCase := output.NewRenderUseCase(output.NewRenderService())
	builderQueryService := builder.NewQueryService(store)
	builderAssembleService := builder.NewAssembleService(store)
	builderConsultUseCase := builder.NewConsultUseCase(store, ragUseCase, aiUseCase, outputUseCase, builderAssembleService, aiclient.DefaultAIRouteForConfig(cfg))
	gatekeeperUseCase := gatekeeper.NewUseCase(gatekeeper.NewGuardService(cfg, store), nil, builderQueryService, builderConsultUseCase)
	gatekeeperHandler := gatekeeper.NewHandler(gatekeeperUseCase)

	mux := http.NewServeMux()
	gatekeeperHandler.Register(mux)

	return &App{
		handler:           withRequestLogging(withPanicRecovery(withCORS(mux, cfg.CORSAllowedOrigins))),
		store:             store,
		gatekeeperUseCase: gatekeeperUseCase,
	}
}
