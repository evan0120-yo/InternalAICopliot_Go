package builder

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestAssemblePromptUsesOverrideAndSkipsUserInputBlock(t *testing.T) {
	service := NewAssembleService(nil)

	result, err := service.AssemblePrompt(
		context.Background(),
		infra.BuilderConfig{
			BuilderID:   1,
			BuilderCode: "pm-estimate",
			GroupLabel:  "產品經理",
			Name:        "PM 工時估算與建議",
			Description: "desc",
		},
		[]infra.Source{
			{SourceID: 10, OrderNo: 1, Prompts: "主要 prompt", NeedsRagSupplement: true},
		},
		map[int64][]infra.RagSupplement{
			10: {
				{RagID: 20, RagType: "default_content", Title: "Default", Content: "舊內容", OrderNo: 1, Overridable: true},
			},
		},
		"",
		"新的需求",
		nil,
	)
	if err != nil {
		t.Fatalf("AssemblePrompt returned error: %v", err)
	}

	if !strings.Contains(result.Instructions, "新的需求") {
		t.Fatalf("expected overridden content in instructions: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "## [USER_INPUT]") {
		t.Fatalf("expected USER_INPUT block to be skipped after override: %s", result.Instructions)
	}
}

func TestAssemblePromptReplacesPlaceholderBeforeFallingBackToSimpleOverride(t *testing.T) {
	service := NewAssembleService(nil)

	result, err := service.AssemblePrompt(
		context.Background(),
		infra.BuilderConfig{
			BuilderID:   1,
			BuilderCode: "pm-estimate",
			GroupLabel:  "產品經理",
			Name:        "PM 工時估算與建議",
			Description: "desc",
		},
		[]infra.Source{
			{SourceID: 10, OrderNo: 1, Prompts: "主要 prompt", NeedsRagSupplement: true},
		},
		map[int64][]infra.RagSupplement{
			10: {
				{
					RagID:       20,
					RagType:     "default_content",
					Title:       "Default",
					Content:     "請依以下需求產出：{{userText}}。最後補上風險提醒。",
					OrderNo:     1,
					Overridable: true,
				},
			},
		},
		"",
		"會員註冊流程",
		nil,
	)
	if err != nil {
		t.Fatalf("AssemblePrompt returned error: %v", err)
	}

	if !strings.Contains(result.Instructions, "請依以下需求產出：會員註冊流程。最後補上風險提醒。") {
		t.Fatalf("expected placeholder replacement in instructions: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "\n### [default_content] Default\n會員註冊流程\n") {
		t.Fatalf("expected Java-style placeholder preservation instead of full replacement: %s", result.Instructions)
	}
}

func TestAssemblePromptReturnsErrorWhenRagMissing(t *testing.T) {
	service := NewAssembleService(nil)

	_, err := service.AssemblePrompt(
		context.Background(),
		infra.BuilderConfig{BuilderID: 1, BuilderCode: "pm-estimate", GroupLabel: "產品經理", Name: "PM 工時估算與建議"},
		[]infra.Source{
			{SourceID: 10, OrderNo: 1, Prompts: "主要 prompt", NeedsRagSupplement: true},
		},
		map[int64][]infra.RagSupplement{},
		"",
		"",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "RAG_SUPPLEMENTS_NOT_FOUND") {
		t.Fatalf("expected missing rag error, got %v", err)
	}
}

func TestAssemblePromptRendersDeterministicSubjectProfileBlock(t *testing.T) {
	service := NewAssembleService(nil)

	result, err := service.AssemblePrompt(
		context.Background(),
		infra.BuilderConfig{
			BuilderID:   1,
			BuilderCode: "pm-estimate",
			GroupLabel:  "產品經理",
			Name:        "PM 工時估算與建議",
			Description: "desc",
		},
		[]infra.Source{
			{SourceID: 10, OrderNo: 1, Prompts: "主要 prompt"},
		},
		map[int64][]infra.RagSupplement{},
		"",
		"請分析這個人",
		&SubjectProfile{
			SubjectID: "user-123",
			AnalysisPayloads: []SubjectAnalysisPayload{
				{
					AnalysisType: "mbti",
					Payload: map[string]any{
						"type":            "INTJ",
						"cognitive_stack": []any{"Ni", `Te|aux`, `Fi\deep`},
					},
				},
				{
					AnalysisType: "astrology",
					Payload: map[string]any{
						"sun_sign":  []any{"Scorpio"},
						"moon_sign": []any{"Pisces"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("AssemblePrompt returned error: %v", err)
	}

	subjectIndex := strings.Index(result.Instructions, "## [SUBJECT_PROFILE]")
	sourceIndex := strings.Index(result.Instructions, "## [SOURCE-1]")
	if subjectIndex < 0 || sourceIndex < 0 || subjectIndex > sourceIndex {
		t.Fatalf("expected SUBJECT_PROFILE block before source block, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "### [analysis:astrology]\nmoon_sign: Pisces\nsun_sign: Scorpio\n") {
		t.Fatalf("expected alphabetically sorted astrology facts, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "### [analysis:mbti]\ncognitive_stack: Ni|Te\\|aux|Fi\\\\deep\ntype: INTJ\n") {
		t.Fatalf("expected mbti values to preserve order and escape separators, got: %s", result.Instructions)
	}
}

func TestAssemblePromptUsesLinkChatStrategyForTheoryMappedModules(t *testing.T) {
	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ResetOnStart:  true,
		SeedWhenEmpty: true,
		SeedData:      infra.DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewAssembleService(store)
	theoryVersion := " astro-v1 "
	builderConfig, ok, err := store.BuilderByIDContext(context.Background(), 3)
	if err != nil {
		t.Fatalf("BuilderByIDContext returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected builder 3 to exist")
	}
	subjectProfile := &SubjectProfile{
		SubjectID: "user-123",
		AnalysisPayloads: []SubjectAnalysisPayload{
			{
				AnalysisType:  "astrology",
				TheoryVersion: &theoryVersion,
				Payload: map[string]any{
					"sun_sign":  []any{"Scorpio"},
					"moon_sign": []any{"雙魚"},
				},
			},
			{
				AnalysisType: "mbti",
				Payload: map[string]any{
					"type": "INTJ",
				},
			},
		},
	}
	sources, err := store.SourcesByBuilderIDContext(context.Background(), 3)
	if err != nil {
		t.Fatalf("SourcesByBuilderIDContext returned error: %v", err)
	}
	filteredSources, err := service.FilterProfileSources(context.Background(), "linkchat", sources, subjectProfile)
	if err != nil {
		t.Fatalf("FilterProfileSources returned error: %v", err)
	}
	result, err := service.AssemblePrompt(
		context.Background(),
		builderConfig,
		filteredSources,
		map[int64][]infra.RagSupplement{},
		"linkchat",
		"請分析這個人",
		subjectProfile,
	)
	if err != nil {
		t.Fatalf("AssemblePrompt returned error: %v", err)
	}

	if !strings.Contains(result.Instructions, "theory_version: astro-v1") {
		t.Fatalf("expected theory version in linkchat subject profile block, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "主執行緒, 發展有好有壞, 主導做事方式和習慣, 以及思維output框架: 深層洞察\n") {
		t.Fatalf("expected sun_sign to resolve primary source prompt and fragment source prompt, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "OS 內核, 主導思維底層邏輯, 包含思維intput架構及運算方式, 喝醉時會同時兼任主執行緒（依照喝醉狀況更多取代本來主執行緒）: 敏感共感\n") {
		t.Fatalf("expected moon_sign to resolve primary source prompt and fragment source prompt, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "type: INTJ\n") {
		t.Fatalf("expected unmapped module to preserve raw value, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "Scorpio") || strings.Contains(result.Instructions, "雙魚") {
		t.Fatalf("did not expect raw theory words to leak into instructions, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "## [THEORY_CODEBOOK]") {
		t.Fatalf("did not expect theory codebook block, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "rising_sign") {
		t.Fatalf("did not expect unrequested primary source to be included, got: %s", result.Instructions)
	}
}
