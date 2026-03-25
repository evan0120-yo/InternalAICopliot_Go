package builder

import "context"
import "com.citrus.internalaicopilot/internal/infra"

// QueryService exposes read-only builder/template graph queries.
type QueryService struct {
	store *infra.Store
}

// NewQueryService builds a read service.
func NewQueryService(store *infra.Store) *QueryService {
	return &QueryService{store: store}
}

// ListActiveBuilders returns the frontend dropdown builders.
func (s *QueryService) ListActiveBuilders(ctx context.Context) ([]BuilderSummary, error) {
	builders, err := s.store.ActiveBuildersContext(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]BuilderSummary, 0, len(builders))
	for _, builderConfig := range builders {
		result = append(result, BuilderSummary{
			BuilderID:           builderConfig.BuilderID,
			BuilderCode:         builderConfig.BuilderCode,
			GroupKey:            builderConfig.GroupKey,
			GroupLabel:          builderConfig.GroupLabel,
			Name:                builderConfig.Name,
			Description:         builderConfig.Description,
			IncludeFile:         builderConfig.IncludeFile,
			DefaultOutputFormat: builderConfig.DefaultOutputFormat,
		})
	}
	return result, nil
}

// LoadGraph returns the builder graph for admin editing.
func (s *QueryService) LoadGraph(ctx context.Context, builderID int) (BuilderGraphResponse, error) {
	builderConfig, ok, err := s.store.BuilderByIDContext(ctx, builderID)
	if err != nil {
		return BuilderGraphResponse{}, err
	}
	if !ok {
		return BuilderGraphResponse{}, infra.NewError("BUILDER_NOT_FOUND", "Requested builder does not exist.", 404)
	}

	sources, err := s.store.SourcesByBuilderIDContext(ctx, builderID)
	if err != nil {
		return BuilderGraphResponse{}, err
	}
	sourceResponses := make([]BuilderGraphSourceResponse, 0, len(sources))
	for _, source := range sources {
		rags, err := s.store.RagsBySourceIDContext(ctx, source.SourceID)
		if err != nil {
			return BuilderGraphResponse{}, err
		}
		ragResponses := make([]BuilderGraphRagResponse, 0, len(rags))
		for _, rag := range rags {
			ragResponses = append(ragResponses, toGraphRagResponse(rag))
		}
		sourceResponses = append(sourceResponses, BuilderGraphSourceResponse{
			SourceID:            source.SourceID,
			TemplateID:          source.CopiedFromTemplateID,
			TemplateKey:         source.CopiedFromTemplateKey,
			TemplateName:        source.CopiedFromTemplateName,
			TemplateDescription: source.CopiedFromTemplateDescription,
			TemplateGroupKey:    source.CopiedFromTemplateGroupKey,
			ModuleKey:           trimStringPtr(&source.ModuleKey),
			OrderNo:             source.OrderNo,
			SystemBlock:         source.SystemBlock,
			Prompts:             source.Prompts,
			Rag:                 ragResponses,
		})
	}

	return BuilderGraphResponse{
		Builder: BuilderGraphBuilderResponse{
			BuilderID:           builderConfig.BuilderID,
			BuilderCode:         builderConfig.BuilderCode,
			GroupKey:            builderConfig.GroupKey,
			GroupLabel:          builderConfig.GroupLabel,
			Name:                builderConfig.Name,
			Description:         builderConfig.Description,
			IncludeFile:         builderConfig.IncludeFile,
			DefaultOutputFormat: builderConfig.DefaultOutputFormat,
			FilePrefix:          builderConfig.FilePrefix,
			Active:              builderConfig.Active,
		},
		Sources: sourceResponses,
	}, nil
}

// ListTemplatesByBuilder returns active templates relevant to one builder.
func (s *QueryService) ListTemplatesByBuilder(ctx context.Context, builderID int) ([]BuilderTemplateResponse, error) {
	builderConfig, ok, err := s.store.BuilderByIDContext(ctx, builderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, infra.NewError("BUILDER_NOT_FOUND", "Requested builder does not exist.", 404)
	}

	templates, err := s.store.TemplatesContext(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]templateWithRags, 0)
	for _, template := range templates {
		if !template.Active {
			continue
		}
		if template.GroupKey != nil {
			if builderConfig.GroupKey == nil || *template.GroupKey != *builderConfig.GroupKey {
				continue
			}
		}
		rags, err := s.store.TemplateRagsByTemplateIDContext(ctx, template.TemplateID)
		if err != nil {
			return nil, err
		}
		filtered = append(filtered, templateWithRags{
			template: template,
			rags:     rags,
		})
	}
	sortTemplates(filtered)
	return toTemplateResponses(filtered), nil
}

// ListAllTemplates returns the full admin template library.
func (s *QueryService) ListAllTemplates(ctx context.Context) ([]BuilderTemplateResponse, error) {
	templates, err := s.store.TemplatesContext(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]templateWithRags, 0, len(templates))
	for _, template := range templates {
		rags, err := s.store.TemplateRagsByTemplateIDContext(ctx, template.TemplateID)
		if err != nil {
			return nil, err
		}
		items = append(items, templateWithRags{
			template: template,
			rags:     rags,
		})
	}
	sortTemplates(items)
	return toTemplateResponses(items), nil
}

type templateWithRags struct {
	template infra.Template
	rags     []infra.TemplateRag
}

func sortTemplates(items []templateWithRags) {
	infra.SortByOrderThenID(items, func(item templateWithRags) int { return item.template.OrderNo }, func(item templateWithRags) int64 { return item.template.TemplateID })
}

func toTemplateResponses(items []templateWithRags) []BuilderTemplateResponse {
	responses := make([]BuilderTemplateResponse, 0, len(items))
	for _, item := range items {
		ragResponses := make([]BuilderTemplateRagResponse, 0, len(item.rags))
		for _, rag := range item.rags {
			ragResponses = append(ragResponses, BuilderTemplateRagResponse{
				TemplateRagID: rag.TemplateRagID,
				RagType:       rag.RagType,
				Title:         rag.Title,
				Content:       rag.Content,
				OrderNo:       rag.OrderNo,
				Overridable:   rag.Overridable,
				RetrievalMode: normalizeRetrievalModeForRead(rag.RetrievalMode),
			})
		}
		responses = append(responses, BuilderTemplateResponse{
			TemplateID:  item.template.TemplateID,
			TemplateKey: item.template.TemplateKey,
			Name:        item.template.Name,
			Description: item.template.Description,
			GroupKey:    item.template.GroupKey,
			OrderNo:     item.template.OrderNo,
			Prompts:     item.template.Prompts,
			Active:      item.template.Active,
			Rag:         ragResponses,
		})
	}
	return responses
}

func toGraphRagResponse(rag infra.RagSupplement) BuilderGraphRagResponse {
	return BuilderGraphRagResponse{
		RagID:         rag.RagID,
		RagType:       rag.RagType,
		Title:         rag.Title,
		Content:       rag.Content,
		OrderNo:       rag.OrderNo,
		Overridable:   rag.Overridable,
		RetrievalMode: normalizeRetrievalModeForRead(rag.RetrievalMode),
	}
}

func normalizeRetrievalModeForRead(raw string) string {
	// Go backend currently exposes a single supported read mode.
	_ = raw
	return "full_context"
}
