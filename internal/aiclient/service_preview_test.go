package aiclient

import (
	"context"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestPreviewBodyTruncatesOnRuneBoundary(t *testing.T) {
	input := []byte(strings.Repeat("測", 600))

	preview := previewBody(input)
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("expected preview suffix, got %q", preview)
	}
	if !strings.Contains(preview, "測") {
		t.Fatalf("expected preview to retain original runes, got %q", preview)
	}
}

func TestAnalyzeReturnsPromptPreviewWhenPreviewModeEnabled(t *testing.T) {
	service := NewAnalyzeService(infra.Config{AIPreviewMode: true})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"user message",
		"assembled instructions",
		[]infra.Attachment{
			{
				FileName:    "birth-chart.png",
				ContentType: "image/png",
				Data:        []byte("12345"),
			},
		},
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if !response.Status || response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected preview response, got %+v", response)
	}
	for _, fragment := range []string{
		`"model": "gpt-4o"`,
		`"instructions": "assembled instructions"`,
		`"text": "user message"`,
		`"preview": true`,
		`"preview_file_name": "birth-chart.png"`,
	} {
		if !strings.Contains(response.Response, fragment) {
			t.Fatalf("expected preview response to contain %q, got %s", fragment, response.Response)
		}
	}
	if strings.Contains(response.Response, `"file_id"`) {
		t.Fatalf("preview should not include uploaded file ids, got %s", response.Response)
	}
}

func TestAnalyzePreviewModeTakesPriorityWhenOpenAIKeyExists(t *testing.T) {
	service := NewAnalyzeService(infra.Config{
		AIPreviewMode: true,
		OpenAIAPIKey:  "sk-test",
	})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"user message",
		"assembled instructions",
		[]infra.Attachment{
			{
				FileName:    "attachment.pdf",
				ContentType: "application/pdf",
				Data:        []byte("abc"),
			},
		},
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected preview mode to win over OpenAI key, got %+v", response)
	}
	if !strings.Contains(response.Response, `"preview_file_name": "attachment.pdf"`) {
		t.Fatalf("expected local attachment summary, got %s", response.Response)
	}
	if strings.Contains(response.Response, `"file_id"`) {
		t.Fatalf("preview should not upload attachments, got %s", response.Response)
	}
}

func TestAnalyzePreviewPreservesStructuredProfileBlocks(t *testing.T) {
	service := NewAnalyzeService(infra.Config{AIPreviewMode: true})
	instructions := `## [SUBJECT_PROFILE]
subject: user-123

### [analysis:astrology]
theory_version: astro-v1
人生主軸: 深層洞察
情緒本能: 敏感共感`

	response, err := service.Analyze(context.Background(), "gpt-4o", "profile text", instructions, nil)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" {
		t.Fatalf("expected prompt preview response, got %+v", response)
	}
	for _, fragment := range []string{
		`"instructions": "## [SUBJECT_PROFILE]`,
		`### [analysis:astrology]`,
		`人生主軸: 深層洞察`,
		`情緒本能: 敏感共感`,
	} {
		if !strings.Contains(response.Response, fragment) {
			t.Fatalf("expected preview response to preserve structured prompt block %q, got %s", fragment, response.Response)
		}
	}
	if strings.Contains(response.Response, "THEORY_CODEBOOK") || strings.Contains(response.Response, "ASTRO_SUN_SCO_01") {
		t.Fatalf("did not expect obsolete codebook content, got %s", response.Response)
	}
}
