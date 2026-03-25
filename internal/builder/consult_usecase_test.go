package builder

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"
)

func TestConsultReturnsBuilderNotFound(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	useCase := NewConsultUseCase(
		store,
		rag.NewResolveUseCase(rag.NewResolveService(store)),
		aiclient.NewAnalyzeUseCase(aiclient.NewAnalyzeService(infra.Config{})),
		output.NewRenderUseCase(output.NewRenderService()),
		NewAssembleService(store),
		"gpt-4o",
	)

	_, consultErr := useCase.Consult(context.Background(), ConsultCommand{BuilderID: 999})
	if consultErr == nil || !strings.Contains(consultErr.Error(), "BUILDER_NOT_FOUND") {
		t.Fatalf("expected builder not found error, got %v", consultErr)
	}
}

func TestConsultReturnsCancelledWhenContextIsDone(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	useCase := NewConsultUseCase(
		store,
		rag.NewResolveUseCase(rag.NewResolveService(store)),
		aiclient.NewAnalyzeUseCase(aiclient.NewAnalyzeService(infra.Config{})),
		output.NewRenderUseCase(output.NewRenderService()),
		NewAssembleService(store),
		"gpt-4o",
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, consultErr := useCase.Consult(ctx, ConsultCommand{BuilderID: 1})
	if consultErr == nil || !strings.Contains(consultErr.Error(), "REQUEST_CANCELLED") {
		t.Fatalf("expected request cancelled error, got %v", consultErr)
	}
}

func TestFilterSourcesForProfileConsultIncludesCommonAndRequestedModules(t *testing.T) {
	sources := []infra.Source{
		{SourceID: 1, ModuleKey: "", Prompts: "common"},
		{SourceID: 2, ModuleKey: "astrology", Prompts: "astrology"},
		{SourceID: 3, ModuleKey: "mbti", Prompts: "mbti"},
	}

	filtered, err := filterSourcesForProfileConsult(sources, []string{"astrology"})
	if err != nil {
		t.Fatalf("filterSourcesForProfileConsult returned error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected common and astrology sources, got %+v", filtered)
	}
	if filtered[0].SourceID != 1 || filtered[1].SourceID != 2 {
		t.Fatalf("unexpected filtered sources: %+v", filtered)
	}
}

func TestFilterSourcesForProfileConsultSupportsTextOnlyProfileRequests(t *testing.T) {
	sources := []infra.Source{
		{SourceID: 1, ModuleKey: "", Prompts: "common"},
		{SourceID: 2, ModuleKey: "astrology", Prompts: "astrology"},
	}

	filtered, err := filterSourcesForProfileConsult(sources, nil)
	if err != nil {
		t.Fatalf("filterSourcesForProfileConsult returned error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].SourceID != 1 {
		t.Fatalf("expected only common sources for text-only profile request, got %+v", filtered)
	}
}

func TestFilterSourcesForProfileConsultRejectsInvalidStoredModuleKey(t *testing.T) {
	sources := []infra.Source{
		{SourceID: 1, ModuleKey: "bad key!", Prompts: "broken"},
	}

	_, err := filterSourcesForProfileConsult(sources, []string{"astrology"})
	if err == nil || !strings.Contains(err.Error(), "INVALID_SOURCE_MODULE_KEY") {
		t.Fatalf("expected invalid stored module key error, got %v", err)
	}
}
