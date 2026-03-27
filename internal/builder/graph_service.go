package builder

import (
	"context"
	"math"
	"regexp"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

var nonGroupKeyPattern = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// GraphService handles builder graph normalization and persistence.
type GraphService struct {
	store *infra.Store
	query *QueryService
}

// NewGraphService builds the graph service.
func NewGraphService(store *infra.Store, query *QueryService) *GraphService {
	return &GraphService{
		store: store,
		query: query,
	}
}

// LoadGraph delegates to the read query.
func (s *GraphService) LoadGraph(ctx context.Context, builderID int) (BuilderGraphResponse, error) {
	return s.query.LoadGraph(ctx, builderID)
}

// SaveGraph persists the admin graph payload.
func (s *GraphService) SaveGraph(ctx context.Context, builderID int, request BuilderGraphRequest) (BuilderGraphResponse, error) {
	existingBuilder, ok, err := s.store.BuilderByIDContext(ctx, builderID)
	if err != nil {
		return BuilderGraphResponse{}, err
	}
	if !ok {
		return BuilderGraphResponse{}, infra.NewError("BUILDER_NOT_FOUND", "Requested builder does not exist.", 404)
	}

	mergedBuilder, err := s.mergeBuilder(ctx, existingBuilder, request.Builder)
	if err != nil {
		return BuilderGraphResponse{}, err
	}
	if _, err := s.store.SaveBuilder(ctx, mergedBuilder); err != nil {
		return BuilderGraphResponse{}, err
	}

	sourceRequests := extractSourceRequests(request)
	normalizedSources, normalizedRags, err := normalizeGraphSources(sourceRequests)
	if err != nil {
		return BuilderGraphResponse{}, err
	}
	for index := range normalizedSources {
		normalizedSources[index].BuilderID = builderID
	}
	if err := s.store.ReplaceBuilderGraph(ctx, builderID, normalizedSources, normalizedRags); err != nil {
		return BuilderGraphResponse{}, err
	}

	return s.query.LoadGraph(ctx, builderID)
}

func (s *GraphService) mergeBuilder(ctx context.Context, existing infra.BuilderConfig, request *BuilderGraphBuilderRequest) (infra.BuilderConfig, error) {
	if request == nil {
		return existing, nil
	}

	if request.BuilderCode != nil {
		builderCode := strings.TrimSpace(*request.BuilderCode)
		if builderCode == "" {
			return infra.BuilderConfig{}, infra.NewError("BUILDER_FIELD_MISSING", "Required builder field is missing.", 400)
		}
		if other, exists, err := s.store.BuilderByCodeContext(ctx, builderCode); err != nil {
			return infra.BuilderConfig{}, err
		} else if exists && other.BuilderID != existing.BuilderID {
			return infra.BuilderConfig{}, infra.NewError("BUILDER_CODE_DUPLICATE", "Builder code already exists.", 400)
		}
		existing.BuilderCode = builderCode
	}
	if request.GroupLabel != nil {
		groupLabel := strings.TrimSpace(*request.GroupLabel)
		if groupLabel != "" {
			existing.GroupLabel = groupLabel
		}
	}
	if request.GroupKey != nil {
		groupKey := strings.TrimSpace(*request.GroupKey)
		if groupKey != "" {
			existing.GroupKey = &groupKey
		}
	}
	if existingGroupKey := trimStringPtr(existing.GroupKey); existingGroupKey != nil {
		existing.GroupKey = existingGroupKey
	} else if derived := toGroupKey(existing.GroupLabel); derived != "" {
		existing.GroupKey = &derived
	}
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return infra.BuilderConfig{}, infra.NewError("BUILDER_FIELD_MISSING", "Required builder field is missing.", 400)
		}
		existing.Name = name
	}
	if request.Description != nil {
		existing.Description = strings.TrimSpace(*request.Description)
	}
	if request.IncludeFile != nil {
		existing.IncludeFile = *request.IncludeFile
	}
	if request.DefaultOutputFormat != nil {
		if strings.TrimSpace(*request.DefaultOutputFormat) == "" {
			existing.DefaultOutputFormat = nil
		} else {
			format, ok := infra.ParseOutputFormat(*request.DefaultOutputFormat)
			if !ok {
				return infra.BuilderConfig{}, infra.NewError("UNSUPPORTED_OUTPUT_FORMAT", "Only markdown and xlsx output formats are supported.", 400)
			}
			value := string(format)
			existing.DefaultOutputFormat = &value
		}
	}
	if request.FilePrefix != nil {
		existing.FilePrefix = strings.TrimSpace(*request.FilePrefix)
	}
	if request.Active != nil {
		existing.Active = *request.Active
	}
	return existing, nil
}

func extractSourceRequests(request BuilderGraphRequest) []BuilderGraphSourceRequest {
	if len(request.Sources) > 0 {
		return request.Sources
	}
	if len(request.AiAgent) == 0 {
		return nil
	}
	result := make([]BuilderGraphSourceRequest, 0, len(request.AiAgent))
	for _, item := range request.AiAgent {
		if item.Source != nil {
			result = append(result, *item.Source)
		}
	}
	return result
}

func normalizeGraphSources(requests []BuilderGraphSourceRequest) ([]infra.Source, []infra.RagSupplement, error) {
	type indexedSource struct {
		index   int
		inputID int64
		request BuilderGraphSourceRequest
	}
	indexed := make([]indexedSource, 0, len(requests))
	seenInputIDs := make(map[int64]struct{}, len(requests))
	for index, request := range requests {
		if request.SystemBlock != nil && *request.SystemBlock {
			continue
		}
		inputID := int64(-(index + 1))
		if request.SourceID != nil && *request.SourceID != 0 {
			inputID = *request.SourceID
		}
		if _, exists := seenInputIDs[inputID]; exists {
			return nil, nil, infra.NewError("SOURCE_ID_DUPLICATE", "Source identifiers must be unique within one graph save request.", 400)
		}
		seenInputIDs[inputID] = struct{}{}
		indexed = append(indexed, indexedSource{index: index, inputID: inputID, request: request})
	}
	infra.SortByOrderThenID(indexed, func(item indexedSource) int {
		if item.request.OrderNo != nil {
			return *item.request.OrderNo
		}
		return math.MaxInt
	}, func(item indexedSource) int64 { return int64(item.index) })

	sources := make([]infra.Source, 0, len(indexed))
	rags := make([]infra.RagSupplement, 0)
	inputToPlaceholder := make(map[int64]int64, len(indexed))
	for sourceIndex, item := range indexed {
		if item.request.OrderNo != nil && *item.request.OrderNo <= 0 {
			return nil, nil, infra.NewError("SOURCE_ORDER_INVALID", "Source orderNo must be positive when provided.", 400)
		}
		sourceType, err := normalizeSourceType(valueOrEmpty(item.request.SourceType))
		if err != nil {
			return nil, nil, err
		}
		matchKey := strings.TrimSpace(valueOrEmpty(item.request.MatchKey))
		placeholderSourceID := int64(-(sourceIndex + 1))
		inputToPlaceholder[item.inputID] = placeholderSourceID

		source := infra.Source{
			SourceID:                      placeholderSourceID,
			Prompts:                       strings.TrimSpace(item.request.Prompts),
			OrderNo:                       sourceIndex + 1,
			SystemBlock:                   false,
			NeedsRagSupplement:            len(item.request.Rag) > 0,
			CopiedFromTemplateID:          item.request.TemplateID,
			CopiedFromTemplateKey:         trimStringPtr(item.request.TemplateKey),
			CopiedFromTemplateName:        trimStringPtr(item.request.TemplateName),
			CopiedFromTemplateDescription: trimStringPtr(item.request.TemplateDescription),
			CopiedFromTemplateGroupKey:    trimStringPtr(item.request.TemplateGroupKey),
			SourceType:                    sourceType,
			MatchKey:                      matchKey,
			Tags:                          normalizeSourceTags(item.request.Tags),
		}
		moduleKey, err := NormalizeStoredModuleKey(valueOrEmpty(item.request.ModuleKey))
		if err != nil {
			return nil, nil, err
		}
		source.ModuleKey = moduleKey
		if len(item.request.SourceIDs) > 0 {
			source.SourceIDs = append([]int64(nil), item.request.SourceIDs...)
		}
		sources = append(sources, source)

		normalizedRags, err := normalizeGraphRags(item.request.Rag, source.SourceID)
		if err != nil {
			return nil, nil, err
		}
		rags = append(rags, normalizedRags...)
	}

	for sourceIndex := range sources {
		if len(sources[sourceIndex].SourceIDs) == 0 {
			continue
		}
		rewritten := make([]int64, 0, len(sources[sourceIndex].SourceIDs))
		for _, requestedID := range sources[sourceIndex].SourceIDs {
			resolvedID, ok := inputToPlaceholder[requestedID]
			if !ok {
				return nil, nil, infra.NewError("SOURCE_REFERENCE_NOT_FOUND", "sourceIds contains an unknown source reference.", 400)
			}
			rewritten = append(rewritten, resolvedID)
		}
		sources[sourceIndex].SourceIDs = rewritten
	}
	return sources, rags, nil
}

func normalizeGraphRags(requests []BuilderGraphRagRequest, placeholderSourceID int64) ([]infra.RagSupplement, error) {
	type indexedRag struct {
		index   int
		request BuilderGraphRagRequest
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

	rags := make([]infra.RagSupplement, 0, len(indexed))
	for index, item := range indexed {
		if item.request.OrderNo != nil && *item.request.OrderNo <= 0 {
			return nil, infra.NewError("RAG_ORDER_INVALID", "RAG orderNo must be positive when provided.", 400)
		}
		if item.request.RagType == nil || strings.TrimSpace(*item.request.RagType) == "" {
			return nil, infra.NewError("RAG_TYPE_MISSING", "RAG type is required.", 400)
		}
		retrievalMode := "full_context"
		if item.request.RetrievalMode != nil && strings.TrimSpace(*item.request.RetrievalMode) != "" {
			if strings.TrimSpace(*item.request.RetrievalMode) != "full_context" {
				return nil, infra.NewError("RAG_RETRIEVAL_MODE_UNSUPPORTED", "Only full_context retrieval mode is currently supported.", 400)
			}
		}
		title := strings.TrimSpace(*item.request.RagType)
		if item.request.Title != nil && strings.TrimSpace(*item.request.Title) != "" {
			title = strings.TrimSpace(*item.request.Title)
		}
		content := strings.TrimSpace(item.request.Content)
		if content == "" {
			content = strings.TrimSpace(item.request.Prompts)
		}
		overridable := false
		if item.request.Overridable != nil {
			overridable = *item.request.Overridable
		}
		rags = append(rags, infra.RagSupplement{
			SourceID:      placeholderSourceID,
			RagType:       strings.TrimSpace(*item.request.RagType),
			Title:         title,
			Content:       content,
			OrderNo:       index + 1,
			Overridable:   overridable,
			RetrievalMode: retrievalMode,
		})
	}
	return rags, nil
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeSourceType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case infra.SourceTypePrimary:
		return infra.SourceTypePrimary, nil
	case infra.SourceTypeFragment:
		return infra.SourceTypeFragment, nil
	default:
		return "", infra.NewError("SOURCE_TYPE_INVALID", "sourceType must be primary or fragment.", 400)
	}
}

func normalizeSourceTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		tag = strings.TrimPrefix(tag, "#")
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func toGroupKey(rawGroupLabel string) string {
	normalized := strings.TrimSpace(rawGroupLabel)
	if normalized == "" {
		return ""
	}
	collapsed := nonGroupKeyPattern.ReplaceAllString(strings.ToLower(normalized), "-")
	collapsed = strings.Trim(collapsed, "-")
	if collapsed == "" {
		return normalized
	}
	return collapsed
}
