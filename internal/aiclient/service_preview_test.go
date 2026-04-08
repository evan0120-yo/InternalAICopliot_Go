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
		"",
		[]infra.Attachment{
			{
				FileName:    "birth-chart.png",
				ContentType: "image/png",
				Data:        []byte("12345"),
			},
		},
		"",
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if !response.Status || response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected preview response, got %+v", response)
	}
	for _, fragment := range []string{
		`## [INSTRUCTIONS]`,
		`assembled instructions`,
		`## [USER_MESSAGE]`,
		`user message`,
		`## [ATTACHMENTS]`,
		`birth-chart.png | image/png | 5 bytes | image`,
	} {
		if !strings.Contains(response.Response, fragment) {
			t.Fatalf("expected preview response to contain %q, got %s", fragment, response.Response)
		}
	}
	for _, obsolete := range []string{`"model":`, `"input":`, `"file_id"`, `"preview_file_name"`} {
		if strings.Contains(response.Response, obsolete) {
			t.Fatalf("preview should only contain text sections, got %s", response.Response)
		}
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
		"",
		[]infra.Attachment{
			{
				FileName:    "attachment.pdf",
				ContentType: "application/pdf",
				Data:        []byte("abc"),
			},
		},
		"",
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected preview mode to win over OpenAI key, got %+v", response)
	}
	if !strings.Contains(response.Response, `attachment.pdf | application/pdf | 3 bytes | file`) {
		t.Fatalf("expected local attachment summary, got %s", response.Response)
	}
	if strings.Contains(response.Response, `"file_id"`) || strings.Contains(response.Response, `"model":`) {
		t.Fatalf("preview should not return OpenAI request JSON, got %s", response.Response)
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

	response, err := service.Analyze(context.Background(), "gpt-4o", "profile text", instructions, "", nil, "")
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" {
		t.Fatalf("expected prompt preview response, got %+v", response)
	}
	for _, fragment := range []string{
		`## [INSTRUCTIONS]`,
		`## [SUBJECT_PROFILE]`,
		`### [analysis:astrology]`,
		`人生主軸: 深層洞察`,
		`情緒本能: 敏感共感`,
		`## [USER_MESSAGE]`,
		`profile text`,
	} {
		if !strings.Contains(response.Response, fragment) {
			t.Fatalf("expected preview response to preserve structured prompt block %q, got %s", fragment, response.Response)
		}
	}
	if strings.Contains(response.Response, "THEORY_CODEBOOK") || strings.Contains(response.Response, "ASTRO_SUN_SCO_01") || strings.Contains(response.Response, `"model":`) {
		t.Fatalf("did not expect obsolete codebook content, got %s", response.Response)
	}
}

func TestAnalyzeReturnsPromptBodyOnlyWhenRequested(t *testing.T) {
	service := NewAnalyzeService(infra.Config{AIPreviewMode: true})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"user message",
		"assembled instructions",
		"OS 內核: 內在敏感\n主執行緒: 50% 魔羯 / 50% 水瓶",
		nil,
		infra.AIExecutionModePreviewPromptBodyOnly,
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected prompt body preview response, got %+v", response)
	}
	if response.Response != "OS 內核: 內在敏感\n主執行緒: 50% 魔羯 / 50% 水瓶" {
		t.Fatalf("expected prompt body only preview, got %q", response.Response)
	}
	for _, forbidden := range []string{"## [INSTRUCTIONS]", "## [USER_MESSAGE]", "assembled instructions"} {
		if strings.Contains(response.Response, forbidden) {
			t.Fatalf("did not expect full preview fragment %q in %q", forbidden, response.Response)
		}
	}
}

func TestAnalyzeLiveModeOverridesGlobalPreviewSwitch(t *testing.T) {
	service := NewAnalyzeService(infra.Config{
		AIPreviewMode: true,
		AIMockMode:    true,
	})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"請依 instructions 執行",
		"## [RAW_USER_TEXT]\n請幫我整理需求\n## [FRAMEWORK_TAIL]",
		"",
		nil,
		infra.AIExecutionModeLive,
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns == "PROMPT_PREVIEW" || response.Preview {
		t.Fatalf("expected live mode to bypass preview response, got %+v", response)
	}
}

func TestAnalyzeUsesConfiguredDefaultModeWhenRequestModeMissing(t *testing.T) {
	service := NewAnalyzeService(infra.Config{
		AIPreviewMode: false,
		AIDefaultMode: infra.AIExecutionModePreviewPromptBodyOnly,
	})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"user message",
		"assembled instructions",
		"OS 內核: 內在敏感\n主執行緒: 50% 魔羯 / 50% 水瓶",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.StatusAns != "PROMPT_PREVIEW" || !response.Preview {
		t.Fatalf("expected configured default mode to return prompt preview, got %+v", response)
	}
	if response.Response != "OS 內核: 內在敏感\n主執行緒: 50% 魔羯 / 50% 水瓶" {
		t.Fatalf("expected prompt body preview from default mode, got %q", response.Response)
	}
}

func TestAnalyzeConfiguredDefaultModeWinsOverLegacyPreviewFlag(t *testing.T) {
	service := NewAnalyzeService(infra.Config{
		AIPreviewMode: true,
		AIDefaultMode: infra.AIExecutionModePreviewPromptBodyOnly,
	})

	response, err := service.Analyze(
		context.Background(),
		"gpt-4o",
		"user message",
		"assembled instructions",
		"主執行緒: prompt body",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if response.Response != "主執行緒: prompt body" {
		t.Fatalf("expected configured default mode to override legacy preview flag, got %q", response.Response)
	}
}
