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

func TestValidateProfileConsultNormalizesModulesAndProfile(t *testing.T) {
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

	_, modules, profile, validationErr := service.ValidateProfileConsult(context.Background(), 1, []string{" MBTI ", "astrology", "mbti"}, &builder.SubjectProfile{
		SubjectID: " user-123 ",
		ModulePayloads: []builder.SubjectModulePayload{
			{
				ModuleKey:     " mbti ",
				TheoryVersion: ptrString(" v1 "),
				Facts: []builder.SubjectFact{
					{FactKey: "type", Values: []string{" INTJ "}},
				},
			},
		},
	}, "", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected profile consult validation success, got %v", validationErr)
	}
	if len(modules) != 2 || modules[0] != "mbti" || modules[1] != "astrology" {
		t.Fatalf("unexpected normalized modules: %+v", modules)
	}
	if profile == nil || profile.SubjectID != "user-123" {
		t.Fatalf("unexpected normalized profile: %+v", profile)
	}
	if profile.ModulePayloads[0].ModuleKey != "mbti" || profile.ModulePayloads[0].Facts[0].Values[0] != "INTJ" {
		t.Fatalf("unexpected normalized profile payload: %+v", profile.ModulePayloads[0])
	}
	if profile.ModulePayloads[0].TheoryVersion == nil || *profile.ModulePayloads[0].TheoryVersion != "v1" {
		t.Fatalf("expected trimmed theory version, got %+v", profile.ModulePayloads[0].TheoryVersion)
	}
}

func TestValidateProfileConsultRejectsReservedCommonModule(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, _, validationErr := service.ValidateProfileConsult(context.Background(), 1, []string{"common"}, nil, "hello", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "RESERVED_MODULE_KEY") {
		t.Fatalf("expected reserved module key error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsDuplicateFactKey(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, _, validationErr := service.ValidateProfileConsult(context.Background(), 1, []string{"mbti"}, &builder.SubjectProfile{
		SubjectID: "user-123",
		ModulePayloads: []builder.SubjectModulePayload{
			{
				ModuleKey: "mbti",
				Facts: []builder.SubjectFact{
					{FactKey: "type", Values: []string{"INTJ"}},
					{FactKey: "type", Values: []string{"ENTJ"}},
				},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "DUPLICATE_FACT_KEY") {
		t.Fatalf("expected duplicate fact key error, got %v", validationErr)
	}
}

func TestValidateProfileConsultRejectsBlankProfileModuleKeyWithProfileContext(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, _, validationErr := service.ValidateProfileConsult(context.Background(), 1, []string{"mbti"}, &builder.SubjectProfile{
		SubjectID: "user-123",
		ModulePayloads: []builder.SubjectModulePayload{
			{
				ModuleKey: " ",
				Facts: []builder.SubjectFact{
					{FactKey: "type", Values: []string{"INTJ"}},
				},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "subjectProfile.modulePayloads.moduleKey") {
		t.Fatalf("expected profile module key context in error, got %v", validationErr)
	}
}

func TestValidateProfileConsultAllowsTextOnlyProfileRequests(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, modules, profile, validationErr := service.ValidateProfileConsult(context.Background(), 1, nil, nil, "只看 common prompt", "127.0.0.1")
	if validationErr != nil {
		t.Fatalf("expected text-only profile request to pass, got %v", validationErr)
	}
	if len(modules) != 0 || profile != nil {
		t.Fatalf("expected no modules and nil profile, got modules=%+v profile=%+v", modules, profile)
	}
}

func TestValidateProfileConsultRejectsBlankTheoryVersionWhenProvided(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewGuardService(infra.Config{}, store)
	_, _, _, validationErr := service.ValidateProfileConsult(context.Background(), 1, []string{"astrology"}, &builder.SubjectProfile{
		SubjectID: "user-123",
		ModulePayloads: []builder.SubjectModulePayload{
			{
				ModuleKey:     "astrology",
				TheoryVersion: ptrString(" "),
				Facts: []builder.SubjectFact{
					{FactKey: "sun_sign", Values: []string{"Scorpio"}},
				},
			},
		},
	}, "", "127.0.0.1")
	if validationErr == nil || !strings.Contains(validationErr.Error(), "THEORY_VERSION_MISSING") {
		t.Fatalf("expected blank theory version error, got %v", validationErr)
	}
}

func ptrString(value string) *string {
	return &value
}
