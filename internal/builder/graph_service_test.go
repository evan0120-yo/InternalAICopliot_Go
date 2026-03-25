package builder

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestSaveGraphNormalizesSourceAndRagOrderAndKeepsSystemBlock(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	response, err := service.SaveGraph(context.Background(), 2, BuilderGraphRequest{
		Sources: []BuilderGraphSourceRequest{
			{
				OrderNo: ptrInt(2),
				Prompts: "第二個 source",
				Rag: []BuilderGraphRagRequest{
					{RagType: ptrString("rag-b"), Content: "內容 B", OrderNo: ptrInt(3)},
					{RagType: ptrString("rag-a"), Content: "內容 A", OrderNo: ptrInt(1)},
				},
			},
			{
				OrderNo: ptrInt(1),
				Prompts: "第一個 source",
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveGraph returned error: %v", err)
	}

	if len(response.Sources) != 3 {
		t.Fatalf("expected 3 sources including system block, got %d", len(response.Sources))
	}
	if !response.Sources[0].SystemBlock {
		t.Fatalf("expected first source to remain system block")
	}
	if response.Sources[1].OrderNo != 1 || response.Sources[1].Prompts != "第一個 source" {
		t.Fatalf("unexpected normalized first source: %+v", response.Sources[1])
	}
	if response.Sources[2].OrderNo != 2 || len(response.Sources[2].Rag) != 2 {
		t.Fatalf("unexpected normalized second source: %+v", response.Sources[2])
	}
	if response.Sources[2].Rag[0].RagType != "rag-a" || response.Sources[2].Rag[0].OrderNo != 1 {
		t.Fatalf("unexpected normalized rag order: %+v", response.Sources[2].Rag)
	}
}

func TestSaveGraphSupportsLegacyRagPromptsAlias(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var request BuilderGraphRequest
	if err := json.Unmarshal([]byte(`{
		"sources": [
			{
				"orderNo": 1,
				"prompts": "主要 prompt",
				"rag": [
					{
						"ragType": "default_content",
						"prompts": "舊欄位內容",
						"orderNo": 1,
						"overridable": true
					}
				]
			}
		]
	}`), &request); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	response, err := service.SaveGraph(context.Background(), 2, request)
	if err != nil {
		t.Fatalf("SaveGraph returned error: %v", err)
	}

	if got := response.Sources[1].Rag[0].Content; got != "舊欄位內容" {
		t.Fatalf("expected legacy prompts alias to populate rag content, got %q", got)
	}
}

func TestSaveGraphDerivesNonNilGroupKeyFromGroupLabel(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.SaveBuilder(context.Background(), infra.BuilderConfig{
		BuilderID:   99,
		BuilderCode: "builder-99",
		GroupLabel:  "QA Team",
		Name:        "Builder 99",
		Description: "desc",
		IncludeFile: false,
		FilePrefix:  "builder-99",
		Active:      true,
	}); err != nil {
		t.Fatalf("SaveBuilder returned error: %v", err)
	}

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	response, err := service.SaveGraph(context.Background(), 99, BuilderGraphRequest{
		Builder: &BuilderGraphBuilderRequest{
			GroupKey:   ptrString(""),
			GroupLabel: ptrString("QA Team"),
			Name:       ptrString("Builder 99"),
		},
		Sources: []BuilderGraphSourceRequest{
			{OrderNo: ptrInt(1), Prompts: "主要 prompt"},
		},
	})
	if err != nil {
		t.Fatalf("SaveGraph returned error: %v", err)
	}

	if response.Builder.GroupKey == nil || *response.Builder.GroupKey != "qa-team" {
		t.Fatalf("expected derived non-nil group key, got %+v", response.Builder.GroupKey)
	}
}

func TestSaveGraphRejectsDuplicateBuilderCode(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	duplicateCode := "pm-estimate"
	_, err = service.SaveGraph(context.Background(), 2, BuilderGraphRequest{
		Builder: &BuilderGraphBuilderRequest{
			BuilderCode: &duplicateCode,
		},
	})
	if err == nil || err.Error() == "" || !strings.Contains(err.Error(), "BUILDER_CODE_DUPLICATE") {
		t.Fatalf("expected duplicate builder code error, got %v", err)
	}
}

func TestSaveGraphRejectsUnsupportedDefaultOutputFormat(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	invalidFormat := "pdf"
	_, err = service.SaveGraph(context.Background(), 2, BuilderGraphRequest{
		Builder: &BuilderGraphBuilderRequest{
			DefaultOutputFormat: &invalidFormat,
		},
	})
	if err == nil || err.Error() == "" || !strings.Contains(err.Error(), "UNSUPPORTED_OUTPUT_FORMAT") {
		t.Fatalf("expected unsupported output format error, got %v", err)
	}
}

func TestSaveGraphNormalizesModuleKeyAndTreatsCommonAsEmpty(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewGraphService(store, query)
	response, err := service.SaveGraph(context.Background(), 2, BuilderGraphRequest{
		Sources: []BuilderGraphSourceRequest{
			{OrderNo: ptrInt(1), Prompts: "common prompt", ModuleKey: ptrString(" common ")},
			{OrderNo: ptrInt(2), Prompts: "mbti prompt", ModuleKey: ptrString(" MBTI ")},
		},
	})
	if err != nil {
		t.Fatalf("SaveGraph returned error: %v", err)
	}

	if response.Sources[1].ModuleKey != nil {
		t.Fatalf("expected common module key to normalize to nil, got %+v", response.Sources[1].ModuleKey)
	}
	if response.Sources[2].ModuleKey == nil || *response.Sources[2].ModuleKey != "mbti" {
		t.Fatalf("expected module key to normalize to mbti, got %+v", response.Sources[2].ModuleKey)
	}
}

func ptrString(value string) *string { return &value }
func ptrInt(value int) *int          { return &value }
