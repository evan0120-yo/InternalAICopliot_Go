package builder

import (
	"context"
	"math"
	"slices"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

// TemplateService handles template CRUD and ordering.
type TemplateService struct {
	store *infra.Store
	query *QueryService
}

// NewTemplateService builds the template service.
func NewTemplateService(store *infra.Store, query *QueryService) *TemplateService {
	return &TemplateService{
		store: store,
		query: query,
	}
}

// ListTemplatesByBuilder returns builder-relevant templates.
func (s *TemplateService) ListTemplatesByBuilder(ctx context.Context, builderID int) ([]BuilderTemplateResponse, error) {
	return s.query.ListTemplatesByBuilder(ctx, builderID)
}

// ListAllTemplates returns all templates.
func (s *TemplateService) ListAllTemplates(ctx context.Context) ([]BuilderTemplateResponse, error) {
	return s.query.ListAllTemplates(ctx)
}

// CreateTemplate persists a new template and canonicalizes order.
func (s *TemplateService) CreateTemplate(ctx context.Context, request BuilderTemplateRequest) (BuilderTemplateResponse, error) {
	template, rags, orderedIDs, err := s.normalizeAndPrepareTemplate(ctx, 0, request, true)
	if err != nil {
		return BuilderTemplateResponse{}, err
	}

	savedTemplate, savedRags, err := s.store.SaveTemplate(ctx, template, rags)
	if err != nil {
		return BuilderTemplateResponse{}, err
	}

	if request.OrderNo != nil {
		orderedIDs = reorderTemplateIDs(orderedIDs, savedTemplate.TemplateID, *request.OrderNo)
	} else {
		orderedIDs = reorderTemplateIDs(orderedIDs, savedTemplate.TemplateID, len(orderedIDs)+1)
	}
	if err := s.store.ReorderTemplates(ctx, orderedIDs); err != nil {
		return BuilderTemplateResponse{}, err
	}

	return s.templateResponseByID(ctx, savedTemplate.TemplateID, savedRags)
}

// UpdateTemplate persists an existing template and canonicalizes order.
func (s *TemplateService) UpdateTemplate(ctx context.Context, templateID int64, request BuilderTemplateRequest) (BuilderTemplateResponse, error) {
	if _, ok, err := s.store.TemplateByIDContext(ctx, templateID); err != nil {
		return BuilderTemplateResponse{}, err
	} else if !ok {
		return BuilderTemplateResponse{}, infra.NewError("TEMPLATE_NOT_FOUND", "Requested template does not exist.", 404)
	}

	template, rags, orderedIDs, err := s.normalizeAndPrepareTemplate(ctx, templateID, request, false)
	if err != nil {
		return BuilderTemplateResponse{}, err
	}

	savedTemplate, savedRags, err := s.store.SaveTemplate(ctx, template, rags)
	if err != nil {
		return BuilderTemplateResponse{}, err
	}

	requestedOrder := savedTemplate.OrderNo
	if request.OrderNo != nil {
		requestedOrder = *request.OrderNo
	}
	orderedIDs = reorderTemplateIDs(orderedIDs, templateID, requestedOrder)
	if err := s.store.ReorderTemplates(ctx, orderedIDs); err != nil {
		return BuilderTemplateResponse{}, err
	}

	return s.templateResponseByID(ctx, savedTemplate.TemplateID, savedRags)
}

// DeleteTemplate removes a template and clears copied references.
func (s *TemplateService) DeleteTemplate(ctx context.Context, templateID int64) error {
	if _, ok, err := s.store.TemplateByIDContext(ctx, templateID); err != nil {
		return err
	} else if !ok {
		return infra.NewError("TEMPLATE_NOT_FOUND", "Requested template does not exist.", 404)
	}
	return s.store.DeleteTemplate(ctx, templateID)
}

func (s *TemplateService) normalizeAndPrepareTemplate(ctx context.Context, templateID int64, request BuilderTemplateRequest, isCreate bool) (infra.Template, []infra.TemplateRag, []int64, error) {
	if request.TemplateKey == nil || strings.TrimSpace(*request.TemplateKey) == "" {
		return infra.Template{}, nil, nil, infra.NewError("TEMPLATE_KEY_MISSING", "Template key is required.", 400)
	}
	if request.Name == nil || strings.TrimSpace(*request.Name) == "" {
		return infra.Template{}, nil, nil, infra.NewError("TEMPLATE_NAME_MISSING", "Template name is required.", 400)
	}
	if request.OrderNo != nil && *request.OrderNo <= 0 {
		return infra.Template{}, nil, nil, infra.NewError("TEMPLATE_ORDER_INVALID", "Template orderNo must be positive when provided.", 400)
	}
	if existingByKey, exists, err := s.store.TemplateByKeyContext(ctx, strings.TrimSpace(*request.TemplateKey)); err != nil {
		return infra.Template{}, nil, nil, err
	} else if exists && existingByKey.TemplateID != templateID {
		return infra.Template{}, nil, nil, infra.NewError("TEMPLATE_KEY_DUPLICATE", "Template key already exists.", 400)
	}

	var existing infra.Template
	if !isCreate {
		var ok bool
		var err error
		existing, ok, err = s.store.TemplateByIDContext(ctx, templateID)
		if err != nil {
			return infra.Template{}, nil, nil, err
		}
		if !ok {
			return infra.Template{}, nil, nil, infra.NewError("TEMPLATE_NOT_FOUND", "Requested template does not exist.", 404)
		}
	}
	template := infra.Template{
		TemplateID:  templateID,
		TemplateKey: strings.TrimSpace(*request.TemplateKey),
		Name:        strings.TrimSpace(*request.Name),
		Description: trimOrEmpty(request.Description),
		GroupKey:    trimStringPtr(request.GroupKey),
		OrderNo:     1,
		Prompts:     trimOrEmpty(request.Prompts),
		Active:      true,
	}
	if !isCreate {
		template.OrderNo = existing.OrderNo
		template.Active = existing.Active
	}
	if request.Active != nil {
		template.Active = *request.Active
	}
	if isCreate {
		templates, err := s.store.TemplatesContext(ctx)
		if err != nil {
			return infra.Template{}, nil, nil, err
		}
		template.OrderNo = len(templates) + 1
	}

	rags, err := normalizeTemplateRags(request.Rag)
	if err != nil {
		return infra.Template{}, nil, nil, err
	}

	existingTemplates, err := s.store.TemplatesContext(ctx)
	if err != nil {
		return infra.Template{}, nil, nil, err
	}
	ordered := make([]int64, 0, len(existingTemplates))
	for _, existingTemplate := range existingTemplates {
		if existingTemplate.TemplateID != templateID {
			ordered = append(ordered, existingTemplate.TemplateID)
		}
	}

	return template, rags, ordered, nil
}

func normalizeTemplateRags(requests []BuilderTemplateRagRequest) ([]infra.TemplateRag, error) {
	type indexedRag struct {
		index   int
		request BuilderTemplateRagRequest
	}
	indexed := make([]indexedRag, 0, len(requests))
	for index, request := range requests {
		indexed = append(indexed, indexedRag{index: index, request: request})
	}
	infra.SortByOrderThenID(indexed, func(item indexedRag) int {
		if item.request.OrderNo != nil {
			return *item.request.OrderNo
		}
		return math.MaxInt
	}, func(item indexedRag) int64 { return int64(item.index) })

	rags := make([]infra.TemplateRag, 0, len(indexed))
	for index, item := range indexed {
		if item.request.OrderNo != nil && *item.request.OrderNo <= 0 {
			return nil, infra.NewError("TEMPLATE_RAG_ORDER_INVALID", "Template RAG orderNo must be positive when provided.", 400)
		}
		if item.request.RagType == nil || strings.TrimSpace(*item.request.RagType) == "" {
			return nil, infra.NewError("TEMPLATE_RAG_TYPE_MISSING", "Template RAG type is required.", 400)
		}
		retrievalMode := "full_context"
		if item.request.RetrievalMode != nil && strings.TrimSpace(*item.request.RetrievalMode) != "" && strings.TrimSpace(*item.request.RetrievalMode) != "full_context" {
			return nil, infra.NewError("RAG_RETRIEVAL_MODE_UNSUPPORTED", "Only full_context retrieval mode is currently supported.", 400)
		}
		title := strings.TrimSpace(*item.request.RagType)
		if item.request.Title != nil && strings.TrimSpace(*item.request.Title) != "" {
			title = strings.TrimSpace(*item.request.Title)
		}
		overridable := false
		if item.request.Overridable != nil {
			overridable = *item.request.Overridable
		}
		rags = append(rags, infra.TemplateRag{
			RagType:       strings.TrimSpace(*item.request.RagType),
			Title:         title,
			Content:       strings.TrimSpace(item.request.Content),
			OrderNo:       index + 1,
			Overridable:   overridable,
			RetrievalMode: retrievalMode,
		})
	}
	return rags, nil
}

func reorderTemplateIDs(existing []int64, targetID int64, requestedOrder int) []int64 {
	result := slices.Clone(existing)
	insertIndex := requestedOrder - 1
	if insertIndex < 0 {
		insertIndex = 0
	}
	if insertIndex > len(result) {
		insertIndex = len(result)
	}
	result = append(result, 0)
	copy(result[insertIndex+1:], result[insertIndex:])
	result[insertIndex] = targetID
	return result
}

func (s *TemplateService) templateResponseByID(ctx context.Context, templateID int64, savedRags []infra.TemplateRag) (BuilderTemplateResponse, error) {
	templates, err := s.query.ListAllTemplates(ctx)
	if err != nil {
		return BuilderTemplateResponse{}, err
	}
	for _, template := range templates {
		if template.TemplateID == templateID {
			if savedRags != nil {
				template.Rag = make([]BuilderTemplateRagResponse, 0, len(savedRags))
				for _, rag := range savedRags {
					template.Rag = append(template.Rag, BuilderTemplateRagResponse{
						TemplateRagID: rag.TemplateRagID,
						RagType:       rag.RagType,
						Title:         rag.Title,
						Content:       rag.Content,
						OrderNo:       rag.OrderNo,
						Overridable:   rag.Overridable,
						RetrievalMode: normalizeRetrievalModeForRead(rag.RetrievalMode),
					})
				}
			}
			return template, nil
		}
	}
	return BuilderTemplateResponse{}, infra.NewError("TEMPLATE_NOT_FOUND", "Requested template does not exist.", 404)
}

func trimOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
