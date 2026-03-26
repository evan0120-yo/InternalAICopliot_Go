package gatekeeper

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
)

func TestValidateConsultRejectsUnsupportedOutputFormat(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}, store)

	_, _, validationErr := service.ValidateConsult(context.Background(), 1, "csv", nil, "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "UNSUPPORTED_OUTPUT_FORMAT") {
		t.Fatalf("expected unsupported output format error, got %v", validationErr)
	}
}

func TestValidateConsultRejectsUnsupportedAttachmentType(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}, store)

	_, _, validationErr := service.ValidateConsult(context.Background(), 1, "", []infra.Attachment{
		{
			FileName: "notes.txt",
			Data:     []byte("hello"),
		},
	}, "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "UNSUPPORTED_FILE_TYPE") {
		t.Fatalf("expected unsupported file type error, got %v", validationErr)
	}
}

func TestValidateExternalAppRejectsMissingAppID(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}, store)

	_, validationErr := service.ValidateExternalApp(context.Background(), "")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "APP_ID_MISSING") {
		t.Fatalf("expected app id missing error, got %v", validationErr)
	}
}

func TestValidateExternalConsultRejectsUnauthorizedBuilder(t *testing.T) {
	seed := infra.DefaultSeedData()
	seed.Apps[0].AllowedBuilderIDs = []int{1}

	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:     testProjectID("gatekeeper-service"),
		ResetOnStart:  true,
		SeedWhenEmpty: true,
		SeedData:      seed,
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}, store)

	_, _, _, validationErr := service.ValidateExternalConsult(context.Background(), "linkchat", 2, "", nil, "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "APP_BUILDER_FORBIDDEN") {
		t.Fatalf("expected app builder forbidden error, got %v", validationErr)
	}
}

func TestValidateProfileConsultNormalizesProfileEnvelope(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}, store)

	_, profile, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: " user-123 ",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{
				AnalysisType:  " MBTI ",
				TheoryVersion: ptrString(" v1 "),
				Payload:       map[string]any{"type": " INTJ "},
			},
		},
	}, "", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected profile consult validation success, got %v", validationErr)
	}
	if profile == nil || profile.SubjectID != "user-123" {
		t.Fatalf("unexpected normalized profile: %+v", profile)
	}
	if profile.AnalysisPayloads[0].AnalysisType != "mbti" {
		t.Fatalf("unexpected normalized profile payload: %+v", profile.AnalysisPayloads[0])
	}
	if profile.AnalysisPayloads[0].Payload["type"] != "INTJ" {
		t.Fatalf("expected trimmed string payload, got %+v", profile.AnalysisPayloads[0].Payload)
	}
	if profile.AnalysisPayloads[0].TheoryVersion == nil || *profile.AnalysisPayloads[0].TheoryVersion != "v1" {
		t.Fatalf("expected trimmed theory version, got %+v", profile.AnalysisPayloads[0].TheoryVersion)
	}
}

func TestValidateProfileConsultRejectsInvalidAnalysisType(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{AnalysisType: " ", Payload: map[string]any{"type": "INTJ"}},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "analysisType") {
		t.Fatalf("expected invalid analysis type error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsDuplicateAnalysisType(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{AnalysisType: "mbti", Payload: map[string]any{"type": "INTJ"}},
			{AnalysisType: "mbti", Payload: map[string]any{"type": "ENTJ"}},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "DUPLICATE_ANALYSIS_PAYLOAD") {
		t.Fatalf("expected duplicate analysis payload error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsBlankAnalysisTypeWithProfileContext(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{
				AnalysisType: " ",
				Payload:      map[string]any{"type": "INTJ"},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "subjectProfile.analysisPayloads.analysisType") {
		t.Fatalf("expected analysis type context in error, got %v", validationErr)
	}
}

func TestValidateProfileConsultAllowsTextOnlyProfileRequests(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, profile, validationErr := service.ValidateProfileConsult(context.Background(), "", 1, nil, "只看 common prompt", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected text-only profile request to pass, got %v", validationErr)
	}
	if profile != nil {
		t.Fatalf("expected nil profile, got profile=%+v", profile)
	}
}

func TestValidateProfileConsultRejectsBlankTheoryVersionWhenProvided(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{
				AnalysisType:  "astrology",
				TheoryVersion: ptrString(" "),
				Payload:       map[string]any{"sun_sign": []any{"Scorpio"}},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "THEORY_VERSION_MISSING") {
		t.Fatalf("expected blank theory version error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRequiresTheoryVersionForLinkChatAstrology(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, validationErr := service.ValidateProfileConsult(context.Background(), "linkchat", 1, &builder.SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []builder.SubjectAnalysisPayload{
			{
				AnalysisType: "astrology",
				Payload:      map[string]any{"sun_sign": []any{"Scorpio"}},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "THEORY_VERSION_REQUIRED") {
		t.Fatalf("expected required theory version error, got %v", validationErr)
	}
}

func ptrString(value string) *string {
	return &value
}
