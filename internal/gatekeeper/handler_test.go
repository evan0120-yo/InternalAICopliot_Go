package gatekeeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"
)

func TestConsultHandlerRejectsInvalidMultipart(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/api/consult", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "INVALID_MULTIPART") {
		t.Fatalf("expected INVALID_MULTIPART response, got %s", recorder.Body.String())
	}
}

func TestConsultHandlerRejectsMissingBuilderID(t *testing.T) {
	handler := newTestHandler(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("text", "hello"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/consult", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "BUILDER_ID_MISSING") {
		t.Fatalf("expected BUILDER_ID_MISSING response, got %s", recorder.Body.String())
	}
}

func TestConsultHandlerReturnsSuccessEnvelope(t *testing.T) {
	handler := newTestHandler(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("builderId", "1"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("text", "請幫我整理需求"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/consult", &body)
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope infra.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope, got %+v", envelope)
	}
}

func TestConsultHandlerAllowsOptionalPromptStrategyAppID(t *testing.T) {
	handler := newTestHandler(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("appId", " linkchat "); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("builderId", "1"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("text", "請幫我整理需求"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/consult", &body)
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope infra.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope, got %+v", envelope)
	}
}

func TestProfileConsultHandlerAllowsPromptStrategyHintWithoutExternalAuth(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/api/profile-consult", strings.NewReader(`{
		"appId":"linkchat",
		"builderId":1,
		"mode":"preview_prompt_body_only",
		"subjectProfile":{
			"subjectId":"user-123",
			"analysisPayloads":[
				{
					"analysisType":"astrology",
					"theoryVersion":"astro-v1",
					"payload":{
						"sun_sign":["Scorpio"],
						"moon_sign":["雙魚"]
					}
				}
			]
		},
		"text":"請分析這個人"
	}`))
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope infra.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope, got %+v", envelope)
	}
}

func TestProfileConsultHandlerRejectsUnsupportedMode(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/api/profile-consult", strings.NewReader(`{
		"appId":"linkchat",
		"builderId":1,
		"mode":"preview_json_only",
		"text":"請分析這個人"
	}`))
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "INVALID_MODE") {
		t.Fatalf("expected INVALID_MODE response, got %s", recorder.Body.String())
	}
}

func TestExternalBuildersHandlerRejectsMissingAppID(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/api/external/builders", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "APP_ID_MISSING") {
		t.Fatalf("expected APP_ID_MISSING response, got %s", recorder.Body.String())
	}
}

func TestExternalBuildersHandlerFiltersAuthorizedBuilders(t *testing.T) {
	seed := infra.DefaultSeedData()
	seed.Apps[0].AllowedBuilderIDs = []int{2}
	handler := newTestHandlerWithSeed(t, seed)

	request := httptest.NewRequest(http.MethodGet, "/api/external/builders", nil)
	request.Header.Set(ExternalAppIDHeader, "linkchat")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope infra.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	dataBytes, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var builders []builder.BuilderSummary
	if err := json.Unmarshal(dataBytes, &builders); err != nil {
		t.Fatalf("Unmarshal builders returned error: %v", err)
	}
	if len(builders) != 1 || builders[0].BuilderID != 2 {
		t.Fatalf("expected only builder 2, got %+v", builders)
	}
}

func TestExternalConsultHandlerRejectsUnauthorizedBuilder(t *testing.T) {
	seed := infra.DefaultSeedData()
	seed.Apps[0].AllowedBuilderIDs = []int{1}
	handler := newTestHandlerWithSeed(t, seed)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("builderId", "2"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("text", "請幫我整理需求"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/external/consult", &body)
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set(ExternalAppIDHeader, "linkchat")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "APP_BUILDER_FORBIDDEN") {
		t.Fatalf("expected APP_BUILDER_FORBIDDEN response, got %s", recorder.Body.String())
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newTestHandlerWithSeed(t, infra.DefaultSeedData())
}

func newTestHandlerWithSeed(t *testing.T, seed infra.StoreSeedData) http.Handler {
	t.Helper()

	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:     testProjectID("gatekeeper-handler"),
		ResetOnStart:  true,
		SeedWhenEmpty: true,
		SeedData:      seed,
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
		AIMockMode:          true,
	}

	builderQuery := builder.NewQueryService(store)
	ragUseCase := rag.NewResolveUseCase(rag.NewResolveService(store))
	aiUseCase := aiclient.NewAnalyzeUseCase(aiclient.NewAnalyzeService(cfg))
	outputUseCase := output.NewRenderUseCase(output.NewRenderService())
	builderConsult := builder.NewConsultUseCase(store, ragUseCase, aiUseCase, outputUseCase, builder.NewAssembleService(store), "gpt-4o")
	useCase := NewUseCase(NewGuardService(cfg, store), builderQuery, builderConsult)

	handler := NewHandler(useCase)
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux
}

func testProjectID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
