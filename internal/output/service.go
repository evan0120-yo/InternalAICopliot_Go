package output

import (
	"encoding/base64"
	"fmt"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

// RenderService applies output policy and renderer selection.
type RenderService struct {
}

// NewRenderService builds the output policy service.
func NewRenderService() *RenderService {
	return &RenderService{}
}

// Render applies file policy and renderer selection.
func (s *RenderService) Render(command RenderCommand) (infra.ConsultBusinessResponse, error) {
	response := command.BusinessResponse
	response.File = nil

	if response.Preview || !response.Status || !command.BuilderConfig.IncludeFile {
		return response, nil
	}

	defaultFormat, err := resolveScenarioDefaultOutputFormat(command.BuilderConfig)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	resolvedFormat := defaultFormat
	if command.OutputFormat != nil {
		resolvedFormat = *command.OutputFormat
	}
	var renderedFile RenderedFile
	switch resolvedFormat {
	case infra.OutputFormatMarkdown:
		renderedFile, err = renderMarkdown(command)
	case infra.OutputFormatXLSX:
		renderedFile, err = renderXLSX(command)
	default:
		err = infra.NewError("UNSUPPORTED_OUTPUT_FORMAT", "Only markdown and xlsx output formats are supported.", 400)
	}
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	response.File = &infra.ConsultFilePayload{
		FileName:    renderedFile.FileName,
		ContentType: renderedFile.ContentType,
		Base64:      base64.StdEncoding.EncodeToString(renderedFile.FileBytes),
	}
	return response, nil
}

func resolveScenarioDefaultOutputFormat(builder infra.BuilderConfig) (infra.OutputFormat, error) {
	if builder.DefaultOutputFormat == nil || strings.TrimSpace(*builder.DefaultOutputFormat) == "" {
		return "", infra.NewError(
			"BUILDER_DEFAULT_OUTPUT_FORMAT_MISSING",
			"Builder requires file output but no default output format is configured.",
			500,
		)
	}

	parsed, ok := infra.ParseOutputFormat(*builder.DefaultOutputFormat)
	if !ok {
		return "", infra.NewError(
			"BUILDER_DEFAULT_OUTPUT_FORMAT_INVALID",
			"Builder default output format is invalid.",
			500,
		)
	}
	return parsed, nil
}

func buildFileName(builder infra.BuilderConfig, extension string) string {
	prefix := builder.FilePrefix
	if prefix == "" {
		prefix = fmt.Sprintf("builder-%d", builder.BuilderID)
	}
	return fmt.Sprintf("%s-consult.%s", prefix, extension)
}
