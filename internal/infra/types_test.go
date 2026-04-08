package infra

import (
	"encoding/json"
	"testing"
)

func TestConsultBusinessResponseJSONIncludesEmptyResponseDetail(t *testing.T) {
	payload, err := json.Marshal(ConsultBusinessResponse{
		Status:         true,
		StatusAns:      "PROMPT_PREVIEW",
		Response:       "preview body",
		ResponseDetail: "",
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if got, ok := decoded["responseDetail"]; !ok || got != "" {
		t.Fatalf("expected empty responseDetail field, got %+v", decoded)
	}
}
