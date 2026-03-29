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
	if !strings.Contains(result.Instructions, "## [EXECUTION_RULES]") {
		t.Fatalf("expected execution rules block at top, got: %s", result.Instructions)
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
	sourceIndex := strings.Index(result.Instructions, "## [PROMPT_BLOCK-1]")
	if subjectIndex < 0 || sourceIndex < 0 || subjectIndex > sourceIndex {
		t.Fatalf("expected SUBJECT_PROFILE block before source block, got: %s", result.Instructions)
	}
	rawIndex := strings.Index(result.Instructions, "## [RAW_USER_TEXT]")
	rulesIndex := strings.Index(result.Instructions, "## [EXECUTION_RULES]")
	if rulesIndex < 0 || rawIndex < 0 || rulesIndex > rawIndex {
		t.Fatalf("expected EXECUTION_RULES block before RAW_USER_TEXT, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "### [analysis:astrology]\nmoon_sign: Pisces\nsun_sign: Scorpio\n") {
		t.Fatalf("expected alphabetically sorted astrology facts, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "### [analysis:mbti]\ncognitive_stack: Ni|Te\\|aux|Fi\\\\deep\ntype: INTJ\n") {
		t.Fatalf("expected mbti values to preserve order and escape separators, got: %s", result.Instructions)
	}
	for _, fragment := range []string{
		"builderId=",
		"builderCode=",
		"服務對象為：",
		"任務名稱：",
		"Source 是主 prompt，RAG 是補充 prompt",
		"subject: user-123",
		"## [FRAMEWORK_TAIL]",
	} {
		if strings.Contains(result.Instructions, fragment) {
			t.Fatalf("did not expect internal metadata fragment %q, got: %s", fragment, result.Instructions)
		}
	}
	for _, fragment := range []string{
		`"response": "給顧客看的最終結果"`,
		`"responseDetail": "內部詳細分析內容"`,
		`"response" 是給顧客看的最終答案`,
		`"responseDetail" 是內部詳細分析區`,
	} {
		if !strings.Contains(result.Instructions, fragment) {
			t.Fatalf("expected response/responseDetail rule fragment %q, got: %s", fragment, result.Instructions)
		}
	}
	if strings.Contains(result.PromptBodyPreview, "## [SUBJECT_PROFILE]") || strings.Contains(result.PromptBodyPreview, "### [analysis:") {
		t.Fatalf("did not expect section headings in prompt body preview, got: %s", result.PromptBodyPreview)
	}
	for _, fragment := range []string{
		"moon_sign: Pisces",
		"sun_sign: Scorpio",
		"cognitive_stack: Ni|Te\\|aux|Fi\\\\deep",
		"type: INTJ",
	} {
		if !strings.Contains(result.PromptBodyPreview, fragment) {
			t.Fatalf("expected prompt body preview to contain %q, got: %s", fragment, result.PromptBodyPreview)
		}
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
				AnalysisType: "astrology",
				Payload: map[string]any{
					"sun_sign":  []any{"scorpio"},
					"moon_sign": []any{"pisces"},
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

	if !strings.Contains(result.Instructions, "主執行緒, 發展有好有壞, 主導做事方式和習慣, 以及思維output框架: 處理焦點在自我源頭，運算內容為主觀價值（情感、抽象邏輯）\\|運算資源消耗容易集中在特殊錨點上（每個人不一樣），模擬想像數據:深度*廣度=20*5\n") {
		t.Fatalf("expected sun_sign to resolve primary source plus zodiac child fragments, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "OS 內核, 主導思維底層邏輯, 包含思維intput架構及運算方式, 喝醉時會同時兼任主執行緒（依照喝醉狀況更多取代本來主執行緒）: 處理焦點在自我源頭，運算內容為主觀價值（情感、抽象邏輯）\\|運算資源消耗容易分散在許多地方(scan掃描式的概念)，模擬想像數據:深度*廣度=5*20\n") {
		t.Fatalf("expected moon_sign to resolve primary source plus zodiac child fragments, got: %s", result.Instructions)
	}
	if !strings.Contains(result.Instructions, "type: INTJ\n") {
		t.Fatalf("expected unmapped module to preserve raw value, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "scorpio") || strings.Contains(result.Instructions, "pisces") {
		t.Fatalf("did not expect raw theory words to leak into instructions, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "## [THEORY_CODEBOOK]") {
		t.Fatalf("did not expect theory codebook block, got: %s", result.Instructions)
	}
	if strings.Contains(result.Instructions, "rising_sign") {
		t.Fatalf("did not expect unrequested primary source to be included, got: %s", result.Instructions)
	}
	for _, fragment := range []string{
		"builderId=",
		"builderCode=",
		"服務對象為：",
		"任務名稱：",
		"Source 是主 prompt，RAG 是補充 prompt",
		"subject: user-123",
		"## [FRAMEWORK_TAIL]",
		"## [SOURCE-",
	} {
		if strings.Contains(result.Instructions, fragment) {
			t.Fatalf("did not expect internal prompt metadata fragment %q, got: %s", fragment, result.Instructions)
		}
	}
	if !strings.Contains(result.Instructions, "## [PROMPT_BLOCK-1]") {
		t.Fatalf("expected prompt blocks to use neutral section labels, got: %s", result.Instructions)
	}
}

func TestAssemblePromptRendersWeightedCanonicalEntriesForLinkChatAstrology(t *testing.T) {
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
				AnalysisType: "astrology",
				Payload: map[string]any{
					"sun_sign": []any{
						map[string]any{"key": "capricorn", "weightPercent": float64(70)},
						map[string]any{"key": "aquarius", "weightPercent": float64(30)},
					},
					"moon_sign": []any{
						map[string]any{"key": "pisces"},
					},
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

	for _, fragment := range []string{
		"70% 處理焦點在自我源頭，運算內容為主關邏輯（if else等實體邏輯）\\|運算資源消耗容易集中在Group內，並對Group邊界設有自定義monitor，反應方式參考處理焦點和運算內容，模擬想像數據:深度*廣度=10*10",
		"30% 處理焦點在外部目標，運算內容為客觀邏輯（if else等實體邏輯）\\|運算資源消耗容易集中在特殊錨點上（每個人不一樣），模擬想像數據:深度*廣度=20*5",
	} {
		if !strings.Contains(result.Instructions, fragment) {
			t.Fatalf("expected weighted fragment %q, got: %s", fragment, result.Instructions)
		}
	}
	if strings.Contains(result.Instructions, "capricorn") || strings.Contains(result.Instructions, "aquarius") {
		t.Fatalf("did not expect raw canonical keys to leak into instructions, got: %s", result.Instructions)
	}
	if strings.Contains(result.PromptBodyPreview, "## [SUBJECT_PROFILE]") || strings.Contains(result.PromptBodyPreview, "### [analysis:") {
		t.Fatalf("did not expect subject-profile headings in prompt body preview, got: %s", result.PromptBodyPreview)
	}
	if strings.Contains(result.PromptBodyPreview, "capricorn") || strings.Contains(result.PromptBodyPreview, "aquarius") {
		t.Fatalf("did not expect raw canonical keys to leak into prompt body preview, got: %s", result.PromptBodyPreview)
	}
}
