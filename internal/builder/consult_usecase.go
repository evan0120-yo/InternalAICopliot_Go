package builder

import (
	"context"
	"strings"
	"sync"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"
)

// ConsultUseCase orchestrates the consult flow.
type ConsultUseCase struct {
	store           *infra.Store
	ragUseCase      *rag.ResolveUseCase
	aiUseCase       *aiclient.AnalyzeUseCase
	outputUseCase   *output.RenderUseCase
	assembleService *AssembleService
	defaultAIRoute  aiclient.AIRouteCode
}

// NewConsultUseCase builds the consult entrypoint.
func NewConsultUseCase(store *infra.Store, ragUseCase *rag.ResolveUseCase, aiUseCase *aiclient.AnalyzeUseCase, outputUseCase *output.RenderUseCase, assembleService *AssembleService, defaultAIRoute aiclient.AIRouteCode) *ConsultUseCase {
	return &ConsultUseCase{
		store:           store,
		ragUseCase:      ragUseCase,
		aiUseCase:       aiUseCase,
		outputUseCase:   outputUseCase,
		assembleService: assembleService,
		defaultAIRoute:  defaultAIRoute,
	}
}

// Consult runs the full builder consult orchestration.
func (u *ConsultUseCase) Consult(ctx context.Context, command ConsultCommand) (infra.ConsultBusinessResponse, error) {
	var (
		builderConfig infra.BuilderConfig
		builderOK     bool
		sources       []infra.Source
	)

	var mu sync.Mutex
	var builderErr error
	var sourceErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	if command.PreloadedBuilder != nil {
		builderConfig = *command.PreloadedBuilder
		builderOK = true
	} else {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			loadedBuilderConfig, ok, err := u.store.BuilderByIDContext(ctx, command.BuilderID)
			if err != nil {
				mu.Lock()
				builderErr = err
				mu.Unlock()
				return
			}
			builderConfig, builderOK = loadedBuilderConfig, ok
			if !builderOK {
				mu.Lock()
				builderErr = infra.NewError("BUILDER_NOT_FOUND", "Requested builder does not exist.", 400)
				mu.Unlock()
			}
		}()
	}
	go func() {
		defer waitGroup.Done()
		loadedSources, err := u.store.SourcesByBuilderIDContext(ctx, command.BuilderID)
		if err != nil {
			mu.Lock()
			sourceErr = err
			mu.Unlock()
			return
		}
		sources = loadedSources
		if len(sources) == 0 {
			mu.Lock()
			sourceErr = infra.NewError("SOURCE_ENTRIES_NOT_FOUND", "No source prompt entries were found for the requested builder.", 500)
			mu.Unlock()
		}
	}()
	waitGroup.Wait()
	if builderErr != nil {
		if infra.IsContextCancelled(builderErr) {
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		return infra.ConsultBusinessResponse{}, builderErr
	}
	if sourceErr != nil {
		if infra.IsContextCancelled(sourceErr) {
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		return infra.ConsultBusinessResponse{}, sourceErr
	}
	if err := ctx.Err(); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}

	if command.Mode == ConsultModeProfile {
		filteredSources, filterErr := u.assembleService.FilterProfileSources(ctx, command.AppID, sources, command.SubjectProfile)
		if filterErr != nil {
			return infra.ConsultBusinessResponse{}, filterErr
		}
		sources = filteredSources
		if len(sources) == 0 {
			return infra.ConsultBusinessResponse{}, infra.NewError("SOURCE_ENTRIES_NOT_FOUND", "No source prompt entries were found for the requested builder.", 500)
		}
	}

	ragsBySourceID := make(map[int64][]infra.RagSupplement)
	var ragWaitGroup sync.WaitGroup
	var ragErr error
	for _, source := range sources {
		if !source.NeedsRagSupplement {
			continue
		}
		ragWaitGroup.Add(1)
		go func(source infra.Source) {
			defer ragWaitGroup.Done()
			rags, err := u.ragUseCase.ResolveBySourceID(ctx, source.SourceID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if ragErr == nil {
					ragErr = err
				}
				return
			}
			if len(rags) == 0 && ragErr == nil {
				ragErr = infra.NewError("RAG_SUPPLEMENTS_NOT_FOUND", "A source entry requires RAG supplements but none were found.", 500)
				return
			}
			ragsBySourceID[source.SourceID] = rags
		}(source)
	}
	ragWaitGroup.Wait()
	if ragErr != nil {
		if infra.IsContextCancelled(ragErr) {
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		return infra.ConsultBusinessResponse{}, ragErr
	}
	if err := ctx.Err(); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}

	userText := command.Text
	intentText := ""
	if command.Mode == ConsultModeProfile {
		userText = strings.TrimSpace(command.UserText)
		intentText = strings.TrimSpace(command.IntentText)
	}

	promptResult, err := u.assembleService.AssemblePrompt(ctx, builderConfig, sources, ragsBySourceID, command.AppID, userText, intentText, command.SubjectProfile)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}

	aiRoute := chooseAIRouteCode(command, builderConfig, u.defaultAIRoute)
	businessResponse, err := u.aiUseCase.Analyze(ctx, aiclient.AnalyzeCommand{
		Route:             aiRoute,
		UserText:          promptResult.UserMessageText,
		Instructions:      promptResult.Instructions,
		PromptBodyPreview: promptResult.PromptBodyPreview,
		Attachments:       command.Attachments,
		Mode:              command.AIExecutionMode,
	})
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	return u.outputUseCase.Render(output.RenderCommand{
		BuilderConfig:    builderConfig,
		OutputFormat:     command.OutputFormat,
		BusinessResponse: businessResponse,
	})
}
