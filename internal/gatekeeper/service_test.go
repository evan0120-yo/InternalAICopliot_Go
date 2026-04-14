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
	}, "", "", "127.0.0.1")
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
	}, "", "", "127.0.0.1")
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
	}, "", "", "127.0.0.1")
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
	}, "", "", "127.0.0.1")
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
	_, profile, validationErr := service.ValidateProfileConsult(context.Background(), "", 1, nil, "只看 common prompt", "", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected userText-only profile request to pass, got %v", validationErr)
	}
	if profile != nil {
		t.Fatalf("expected nil profile, got profile=%+v", profile)
	}
}

func TestValidateProfileConsultAllowsIntentTextOnlyProfileRequests(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, profile, validationErr := service.ValidateProfileConsult(context.Background(), "", 1, nil, "", "請分析這個人的核心性格與外在社交表現", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected intentText-only profile request to pass, got %v", validationErr)
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
	}, "", "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "THEORY_VERSION_MISSING") {
		t.Fatalf("expected blank theory version error, got %v", validationErr)
	}
}

func TestValidateProfileConsultAllowsMissingTheoryVersionForLinkChatAstrology(t *testing.T) {
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
	}, "", "", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected missing theoryVersion to be allowed, got %v", validationErr)
	}
}

func TestValidateProfileConsultAllowsSingleWeightedEntryWithoutWeightPercent(t *testing.T) {
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
				Payload: map[string]any{
					"sun_sign": []any{
						map[string]any{"key": "capricorn"},
					},
				},
			},
		},
	}, "", "", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected single weighted entry without weightPercent to pass, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsMissingWeightPercentWhenMultipleEntriesProvided(t *testing.T) {
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
				Payload: map[string]any{
					"sun_sign": []any{
						map[string]any{"key": "capricorn", "weightPercent": float64(70)},
						map[string]any{"key": "aquarius"},
					},
				},
			},
		},
	}, "", "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "weightPercent") {
		t.Fatalf("expected missing weightPercent error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsWeightPercentTotalNotEqualHundred(t *testing.T) {
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
				Payload: map[string]any{
					"sun_sign": []any{
						map[string]any{"key": "capricorn", "weightPercent": float64(60)},
						map[string]any{"key": "aquarius", "weightPercent": float64(30)},
					},
				},
			},
		},
	}, "", "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "equal 100") {
		t.Fatalf("expected weightPercent total error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsWeightedEntryWithoutKey(t *testing.T) {
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
				Payload: map[string]any{
					"sun_sign": []any{
						map[string]any{"weightPercent": float64(100)},
					},
				},
			},
		},
	}, "", "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), ".key") {
		t.Fatalf("expected missing key error, got %v", validationErr)
	}
}

func TestValidateLineTaskConsultRequiresMessageReferenceTimeAndTimeZone(t *testing.T) {
	service := NewGuardService(infra.Config{}, nil)

	if _, validationErr := service.ValidateLineTaskConsult(context.Background(), 1, "", "2026-04-14 10:00:00", "Asia/Taipei", "127.0.0.1"); validationErr == nil || !strings.Contains(validationErr.Error(), "LINE_TASK_MESSAGE_TEXT_MISSING") {
		t.Fatalf("expected messageText missing error, got %v", validationErr)
	}
	if _, validationErr := service.ValidateLineTaskConsult(context.Background(), 1, "小傑 明天 下午三點找我吃飯", "", "Asia/Taipei", "127.0.0.1"); validationErr == nil || !strings.Contains(validationErr.Error(), "LINE_TASK_REFERENCE_TIME_MISSING") {
		t.Fatalf("expected referenceTime missing error, got %v", validationErr)
	}
	if _, validationErr := service.ValidateLineTaskConsult(context.Background(), 1, "小傑 明天 下午三點找我吃飯", "2026-04-14 10:00:00", "", "127.0.0.1"); validationErr == nil || !strings.Contains(validationErr.Error(), "LINE_TASK_TIME_ZONE_MISSING") {
		t.Fatalf("expected timeZone missing error, got %v", validationErr)
	}
}

func ptrString(value string) *string {
	return &value
}
