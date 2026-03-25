package infra

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONStrictRejectsOversizedBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/admin/templates", strings.NewReader(`{"name":"`+strings.Repeat("x", 128)+`"}`))
	recorder := httptest.NewRecorder()

	var payload map[string]any
	err := DecodeJSONStrict(recorder, request, &payload, 32)
	if err == nil || !strings.Contains(err.Error(), "REQUEST_BODY_TOO_LARGE") {
		t.Fatalf("expected body too large error, got %v", err)
	}
}
