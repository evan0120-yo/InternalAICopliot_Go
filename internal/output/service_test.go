package output

import (
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestRenderReturnsErrorWhenBuilderDefaultMissing(t *testing.T) {
	service := NewRenderService()

	_, err := service.Render(RenderCommand{
		BuilderConfig: infra.BuilderConfig{
			BuilderID:   1,
			IncludeFile: true,
			FilePrefix:  "pm-estimate",
		},
		BusinessResponse: infra.ConsultBusinessResponse{
			Status:   true,
			Response: "hello",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "BUILDER_DEFAULT_OUTPUT_FORMAT_MISSING") {
		t.Fatalf("expected missing default output format error, got %v", err)
	}
}

func TestRenderReturnsErrorWhenBuilderDefaultInvalid(t *testing.T) {
	service := NewRenderService()
	invalid := "pdf"

	_, err := service.Render(RenderCommand{
		BuilderConfig: infra.BuilderConfig{
			BuilderID:           1,
			IncludeFile:         true,
			DefaultOutputFormat: &invalid,
			FilePrefix:          "pm-estimate",
		},
		BusinessResponse: infra.ConsultBusinessResponse{
			Status:   true,
			Response: "hello",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "BUILDER_DEFAULT_OUTPUT_FORMAT_INVALID") {
		t.Fatalf("expected invalid default output format error, got %v", err)
	}
}

func TestRenderMarkdownRenderer(t *testing.T) {
	file, err := renderMarkdown(RenderCommand{
		BuilderConfig: infra.BuilderConfig{BuilderID: 1, FilePrefix: "pm-estimate"},
		BusinessResponse: infra.ConsultBusinessResponse{
			Status:   true,
			Response: "# title",
		},
	})
	if err != nil {
		t.Fatalf("renderMarkdown returned error: %v", err)
	}
	if file.ContentType != "text/markdown; charset=utf-8" || string(file.FileBytes) != "# title" {
		t.Fatalf("unexpected markdown file: %+v", file)
	}
}
