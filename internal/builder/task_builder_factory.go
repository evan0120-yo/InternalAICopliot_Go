package builder

import (
	"context"
	"strings"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
)

type consultTaskBuildInput struct {
	Command        ConsultCommand
	BuilderConfig  infra.BuilderConfig
	Sources        []infra.Source
	RagsBySourceID map[int64][]infra.RagSupplement
}

type consultTaskBuildResult struct {
	Route            aiclient.AIRouteCode
	ResponseContract aiclient.AnalyzeResponseContract
	Prompt           promptAssemblyResult
}

type consultTaskBuilder interface {
	PrepareSources(ctx context.Context, assembleService *AssembleService, command ConsultCommand, sources []infra.Source) ([]infra.Source, error)
	Build(ctx context.Context, assembleService *AssembleService, input consultTaskBuildInput) (consultTaskBuildResult, error)
}

type consultTaskBuilderFactory struct {
	defaultAIRoute aiclient.AIRouteCode
}

func newConsultTaskBuilderFactory(defaultAIRoute aiclient.AIRouteCode) consultTaskBuilderFactory {
	return consultTaskBuilderFactory{defaultAIRoute: defaultAIRoute}
}

func (f consultTaskBuilderFactory) BuilderFor(command ConsultCommand) consultTaskBuilder {
	switch command.Mode {
	case ConsultModeProfile:
		return profileConsultTaskBuilder{defaultAIRoute: f.defaultAIRoute}
	case ConsultModeExtract:
		return extractConsultTaskBuilder{defaultAIRoute: f.defaultAIRoute}
	default:
		return genericConsultTaskBuilder{defaultAIRoute: f.defaultAIRoute}
	}
}

type genericConsultTaskBuilder struct {
	defaultAIRoute aiclient.AIRouteCode
}

func (b genericConsultTaskBuilder) PrepareSources(_ context.Context, _ *AssembleService, _ ConsultCommand, sources []infra.Source) ([]infra.Source, error) {
	return sources, nil
}

func (b genericConsultTaskBuilder) Build(ctx context.Context, assembleService *AssembleService, input consultTaskBuildInput) (consultTaskBuildResult, error) {
	prompt, err := assembleService.AssemblePrompt(
		ctx,
		input.BuilderConfig,
		input.Sources,
		input.RagsBySourceID,
		input.Command.AppID,
		strings.TrimSpace(input.Command.Text),
		"",
		input.Command.SubjectProfile,
	)
	if err != nil {
		return consultTaskBuildResult{}, err
	}
	return consultTaskBuildResult{
		Route:            chooseAIRouteCode(input.Command, input.BuilderConfig, b.defaultAIRoute),
		ResponseContract: aiclient.AnalyzeResponseContractConsult,
		Prompt:           prompt,
	}, nil
}

type profileConsultTaskBuilder struct {
	defaultAIRoute aiclient.AIRouteCode
}

func (b profileConsultTaskBuilder) PrepareSources(ctx context.Context, assembleService *AssembleService, command ConsultCommand, sources []infra.Source) ([]infra.Source, error) {
	return assembleService.FilterProfileSources(ctx, command.AppID, sources, command.SubjectProfile)
}

func (b profileConsultTaskBuilder) Build(ctx context.Context, assembleService *AssembleService, input consultTaskBuildInput) (consultTaskBuildResult, error) {
	prompt, err := assembleService.AssemblePrompt(
		ctx,
		input.BuilderConfig,
		input.Sources,
		input.RagsBySourceID,
		input.Command.AppID,
		strings.TrimSpace(input.Command.UserText),
		strings.TrimSpace(input.Command.IntentText),
		input.Command.SubjectProfile,
	)
	if err != nil {
		return consultTaskBuildResult{}, err
	}
	return consultTaskBuildResult{
		Route:            chooseAIRouteCode(input.Command, input.BuilderConfig, b.defaultAIRoute),
		ResponseContract: aiclient.AnalyzeResponseContractConsult,
		Prompt:           prompt,
	}, nil
}

type extractConsultTaskBuilder struct {
	defaultAIRoute aiclient.AIRouteCode
}

func (b extractConsultTaskBuilder) PrepareSources(_ context.Context, _ *AssembleService, _ ConsultCommand, sources []infra.Source) ([]infra.Source, error) {
	return sources, nil
}

func (b extractConsultTaskBuilder) Build(ctx context.Context, assembleService *AssembleService, input consultTaskBuildInput) (consultTaskBuildResult, error) {
	prompt, err := assembleService.AssembleExtractPrompt(
		ctx,
		input.BuilderConfig,
		input.Sources,
		input.RagsBySourceID,
		strings.TrimSpace(input.Command.Text),
		strings.TrimSpace(input.Command.ReferenceTime),
		strings.TrimSpace(input.Command.TimeZone),
	)
	if err != nil {
		return consultTaskBuildResult{}, err
	}
	return consultTaskBuildResult{
		Route:            chooseAIRouteCode(input.Command, input.BuilderConfig, b.defaultAIRoute),
		ResponseContract: aiclient.AnalyzeResponseContractExtraction,
		Prompt:           prompt,
	}, nil
}
