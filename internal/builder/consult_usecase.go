package builder

import (
	"context"
	"log"
	"sync"
	"time"

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
	start := time.Now()
	log.Printf("builder consult start: mode=%d builderID=%d appID=%q clientIP=%q", command.Mode, command.BuilderID, command.AppID, command.ClientIP)

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
			log.Printf("builder consult cancelled at store load: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		log.Printf("builder consult store load failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), builderErr)
		return infra.ConsultBusinessResponse{}, builderErr
	}
	if sourceErr != nil {
		if infra.IsContextCancelled(sourceErr) {
			log.Printf("builder consult cancelled at store load: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		log.Printf("builder consult source load failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), sourceErr)
		return infra.ConsultBusinessResponse{}, sourceErr
	}
	if err := ctx.Err(); err != nil {
		log.Printf("builder consult cancelled at store load: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}
	log.Printf("builder consult store loaded: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())

	taskBuilder := newConsultTaskBuilderFactory(u.defaultAIRoute).BuilderFor(command)
	filteredSources, err := taskBuilder.PrepareSources(ctx, u.assembleService, command, sources)
	if err != nil {
		log.Printf("builder consult prepare sources failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), err)
		return infra.ConsultBusinessResponse{}, err
	}
	sources = filteredSources
	if len(sources) == 0 {
		log.Printf("builder consult no sources: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
		return infra.ConsultBusinessResponse{}, infra.NewError("SOURCE_ENTRIES_NOT_FOUND", "No source prompt entries were found for the requested builder.", 500)
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
			log.Printf("builder consult cancelled at rag: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
			return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
		}
		log.Printf("builder consult rag failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), ragErr)
		return infra.ConsultBusinessResponse{}, ragErr
	}
	if err := ctx.Err(); err != nil {
		log.Printf("builder consult cancelled at rag: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}
	log.Printf("builder consult rag loaded: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())

	buildResult, err := taskBuilder.Build(ctx, u.assembleService, consultTaskBuildInput{
		Command:        command,
		BuilderConfig:  builderConfig,
		Sources:        sources,
		RagsBySourceID: ragsBySourceID,
	})
	if err != nil {
		log.Printf("builder consult prompt build failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), err)
		return infra.ConsultBusinessResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		log.Printf("builder consult cancelled after prompt build: mode=%d builderID=%d elapsed_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
		return infra.ConsultBusinessResponse{}, infra.NewError("REQUEST_CANCELLED", "Request was cancelled.", 499)
	}
	log.Printf("builder consult prompt built: mode=%d builderID=%d route=%s elapsed_ms=%d", command.Mode, command.BuilderID, buildResult.Route, time.Since(start).Milliseconds())

	businessResponse, err := u.aiUseCase.Analyze(ctx, aiclient.AnalyzeCommand{
		Route:             buildResult.Route,
		ResponseContract:  buildResult.ResponseContract,
		UserText:          buildResult.Prompt.UserMessageText,
		Instructions:      buildResult.Prompt.Instructions,
		PromptBodyPreview: buildResult.Prompt.PromptBodyPreview,
		Attachments:       command.Attachments,
		Mode:              command.AIExecutionMode,
	})
	if err != nil {
		log.Printf("builder consult ai failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), err)
		return infra.ConsultBusinessResponse{}, err
	}

	result, err := u.outputUseCase.Render(output.RenderCommand{
		BuilderConfig:    builderConfig,
		OutputFormat:     command.OutputFormat,
		BusinessResponse: businessResponse,
	})
	if err != nil {
		log.Printf("builder consult render failed: mode=%d builderID=%d elapsed_ms=%d err=%v", command.Mode, command.BuilderID, time.Since(start).Milliseconds(), err)
		return infra.ConsultBusinessResponse{}, err
	}
	log.Printf("builder consult completed: mode=%d builderID=%d duration_ms=%d", command.Mode, command.BuilderID, time.Since(start).Milliseconds())
	return result, nil
}
