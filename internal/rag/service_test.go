package rag

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestResolveBySourceIDNormalizesRetrievalModeToFullContext(t *testing.T) {
	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:    "internal-ai-copilot-rag-normalize",
		EmulatorHost: "localhost:8090",
		ResetOnStart: true,
		SeedData: infra.StoreSeedData{
			Builders: []infra.BuilderConfig{
				{
					BuilderID:   1,
					BuilderCode: "builder-1",
					GroupLabel:  "QA",
					Name:        "Builder 1",
					Description: "desc",
					IncludeFile: false,
					FilePrefix:  "builder-1",
					Active:      true,
				},
			},
			Sources: []infra.Source{
				{
					SourceID:           10,
					BuilderID:          1,
					Prompts:            "prompt",
					OrderNo:            1,
					SystemBlock:        false,
					NeedsRagSupplement: true,
				},
			},
			Rags: []infra.RagSupplement{
				{
					RagID:         20,
					SourceID:      10,
					RagType:       "default_content",
					Title:         "Default",
					Content:       "內容",
					OrderNo:       1,
					Overridable:   false,
					RetrievalMode: "vector_search",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewResolveService(store)
	rags, err := service.ResolveBySourceID(context.Background(), 10)
	if err != nil {
		t.Fatalf("ResolveBySourceID returned error: %v", err)
	}
	if len(rags) != 1 || rags[0].RetrievalMode != "full_context" {
		t.Fatalf("expected retrieval mode to normalize to full_context, got %+v", rags)
	}
}

func TestResolveBySourceIDReturnsContextCancellation(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewResolveService(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = service.ResolveBySourceID(ctx, 6)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
