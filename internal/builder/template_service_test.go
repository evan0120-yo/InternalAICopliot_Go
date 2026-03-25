package builder

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestCreateAndDeleteTemplateMaintainsOrderAndClearsReferences(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)
	created, err := service.CreateTemplate(context.Background(), BuilderTemplateRequest{
		TemplateKey: ptrString("qa-template"),
		Name:        ptrString("QA 範本"),
		GroupKey:    ptrString("qa"),
		OrderNo:     ptrInt(1),
		Prompts:     ptrString("主要 prompts"),
		Rag: []BuilderTemplateRagRequest{
			{RagType: ptrString("default_content"), Content: "內容", OrderNo: ptrInt(1)},
		},
	})
	if err != nil {
		t.Fatalf("CreateTemplate returned error: %v", err)
	}
	if created.TemplateKey != "qa-template" || created.OrderNo != 1 {
		t.Fatalf("unexpected created template: %+v", created)
	}

	if err := service.DeleteTemplate(context.Background(), created.TemplateID); err != nil {
		t.Fatalf("DeleteTemplate returned error: %v", err)
	}

	templates, err := service.ListAllTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListAllTemplates returned error: %v", err)
	}
	for _, template := range templates {
		if template.TemplateID == created.TemplateID {
			t.Fatalf("deleted template still present: %+v", template)
		}
	}
}

func TestUpdateTemplateReordersExistingTemplate(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)

	templatesBefore, err := service.ListAllTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListAllTemplates returned error: %v", err)
	}
	target := templatesBefore[len(templatesBefore)-1]

	updated, err := service.UpdateTemplate(context.Background(), target.TemplateID, BuilderTemplateRequest{
		TemplateKey: &target.TemplateKey,
		Name:        &target.Name,
		Description: &target.Description,
		GroupKey:    target.GroupKey,
		OrderNo:     ptrInt(1),
		Prompts:     &target.Prompts,
		Active:      &target.Active,
	})
	if err != nil {
		t.Fatalf("UpdateTemplate returned error: %v", err)
	}
	if updated.TemplateID != target.TemplateID {
		t.Fatalf("unexpected updated template: %+v", updated)
	}

	templatesAfter, err := service.ListAllTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListAllTemplates returned error: %v", err)
	}
	if templatesAfter[0].TemplateID != target.TemplateID || templatesAfter[0].OrderNo != 1 {
		t.Fatalf("expected updated template to move to first position, got %+v", templatesAfter[0])
	}
}

func TestCreateTemplateRejectsMissingName(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)
	_, err = service.CreateTemplate(context.Background(), BuilderTemplateRequest{
		TemplateKey: ptrString("missing-name"),
	})
	if err == nil || !strings.Contains(err.Error(), "TEMPLATE_NAME_MISSING") {
		t.Fatalf("expected missing name error, got %v", err)
	}
}

func TestCreateTemplateRejectsDuplicateKey(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)
	_, err = service.CreateTemplate(context.Background(), BuilderTemplateRequest{
		TemplateKey: ptrString("system-guard"),
		Name:        ptrString("Duplicate"),
	})
	if err == nil || !strings.Contains(err.Error(), "TEMPLATE_KEY_DUPLICATE") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestCreateTemplateRejectsUnsupportedRetrievalMode(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)
	unsupportedMode := "vector_search"
	_, err = service.CreateTemplate(context.Background(), BuilderTemplateRequest{
		TemplateKey: ptrString("bad-retrieval"),
		Name:        ptrString("Bad Retrieval"),
		Rag: []BuilderTemplateRagRequest{
			{
				RagType:       ptrString("default_content"),
				Content:       "內容",
				RetrievalMode: &unsupportedMode,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "RAG_RETRIEVAL_MODE_UNSUPPORTED") {
		t.Fatalf("expected unsupported retrieval mode error, got %v", err)
	}
}

func TestCreateTemplateRejectsMissingRagType(t *testing.T) {
	store, err := infra.NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	query := NewQueryService(store)
	service := NewTemplateService(store, query)
	_, err = service.CreateTemplate(context.Background(), BuilderTemplateRequest{
		TemplateKey: ptrString("missing-rag-type"),
		Name:        ptrString("Missing Rag Type"),
		Rag: []BuilderTemplateRagRequest{
			{
				Content: "內容",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "TEMPLATE_RAG_TYPE_MISSING") {
		t.Fatalf("expected missing rag type error, got %v", err)
	}
}
