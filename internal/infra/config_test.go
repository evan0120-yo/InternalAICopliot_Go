package infra

import "testing"

func TestLoadConfigFromEnvUsesAIProfileForLiveOpenAI(t *testing.T) {
	t.Setenv(EnvAIProfile, "4")
	t.Setenv("INTERNAL_AI_COPILOT_AI_DEFAULT_MODE", string(AIExecutionModePreviewFull))
	t.Setenv("INTERNAL_AI_COPILOT_AI_MOCK_MODE", "true")
	t.Setenv("INTERNAL_AI_COPILOT_AI_PROVIDER", string(AIProviderGemma))
	t.Setenv("INTERNAL_AI_COPILOT_AI_MODEL", "legacy-openai-model")
	t.Setenv("OPENAI_BASE_URL", "https://legacy-openai.example.com")

	config := LoadConfigFromEnv()

	if config.AIProfile.ID != 4 {
		t.Fatalf("expected profile 4, got %+v", config.AIProfile)
	}
	if config.AIDefaultMode != AIExecutionModeLive || config.AIMockMode || config.AIProvider != AIProviderOpenAI {
		t.Fatalf("expected profile 4 to resolve live/openai/non-mock, got %+v", config)
	}
	if config.OpenAIModel != DefaultOpenAIModel || config.OpenAIBaseURL != DefaultOpenAIBaseURL {
		t.Fatalf("expected profile 4 openai defaults, got %+v", config)
	}
}

func TestLoadConfigFromEnvUsesAIProfileForLiveGemma(t *testing.T) {
	t.Setenv(EnvAIProfile, "5")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("INTERNAL_AI_COPILOT_GEMMA_MODEL", "legacy-gemma-model")
	t.Setenv("INTERNAL_AI_COPILOT_GEMMA_BASE_URL", "https://legacy-gemma.example.com")

	config := LoadConfigFromEnv()

	if config.AIProfile.ID != 5 {
		t.Fatalf("expected profile 5, got %+v", config.AIProfile)
	}
	if config.AIProvider != AIProviderGemma || config.AIDefaultMode != AIExecutionModeLive || config.AIMockMode {
		t.Fatalf("expected profile 5 to resolve live/gemma/non-mock, got %+v", config)
	}
	if config.GemmaModel != DefaultGemmaModel || config.GemmaBaseURL != DefaultGemmaBaseURL {
		t.Fatalf("expected profile 5 gemma defaults, got %+v", config)
	}
	if config.GemmaAPIKey != "gemini-key" {
		t.Fatalf("expected gemini api key fallback, got %+v", config)
	}
}

func TestLoadConfigFromEnvFallsBackToLegacyConfigWhenAIProfileMissing(t *testing.T) {
	t.Setenv(EnvAIProfile, "")
	t.Setenv("INTERNAL_AI_COPILOT_AI_DEFAULT_MODE", string(AIExecutionModePreviewPromptBodyOnly))
	t.Setenv("INTERNAL_AI_COPILOT_AI_MOCK_MODE", "true")
	t.Setenv("INTERNAL_AI_COPILOT_AI_PROVIDER", string(AIProviderGemma))
	t.Setenv("INTERNAL_AI_COPILOT_GEMMA_MODEL", "legacy-gemma-model")
	t.Setenv("INTERNAL_AI_COPILOT_GEMMA_BASE_URL", "https://legacy-gemma.example.com")

	config := LoadConfigFromEnv()

	if config.AIProfile.ID != 0 {
		t.Fatalf("expected missing profile to keep AIProfile empty, got %+v", config.AIProfile)
	}
	if config.AIDefaultMode != AIExecutionModePreviewPromptBodyOnly || !config.AIMockMode || config.AIProvider != AIProviderGemma {
		t.Fatalf("expected legacy env fallback, got %+v", config)
	}
	if config.GemmaModel != "legacy-gemma-model" || config.GemmaBaseURL != "https://legacy-gemma.example.com" {
		t.Fatalf("expected legacy gemma overrides, got %+v", config)
	}
}

func TestLoadConfigFromEnvPrefersGeminiAPIKeyOverLegacyGemmaKeys(t *testing.T) {
	t.Setenv(EnvAIProfile, "5")
	t.Setenv("GEMINI_API_KEY", "gemini-primary")
	t.Setenv("INTERNAL_AI_COPILOT_GEMMA_API_KEY", "legacy-gemma")
	t.Setenv("REWARDBRIDGE_GEMMA_API_KEY", "legacy-rewardbridge")

	config := LoadConfigFromEnv()
	if config.GemmaAPIKey != "gemini-primary" {
		t.Fatalf("expected GEMINI_API_KEY to win, got %+v", config)
	}
}

func TestResolvedAIModelUsesSharedDefaultConstants(t *testing.T) {
	if model := (Config{AIProvider: AIProviderOpenAI}).ResolvedAIModel(); model != DefaultOpenAIModel {
		t.Fatalf("expected openai default model %q, got %q", DefaultOpenAIModel, model)
	}
	if model := (Config{AIProvider: AIProviderGemma}).ResolvedAIModel(); model != DefaultGemmaModel {
		t.Fatalf("expected gemma default model %q, got %q", DefaultGemmaModel, model)
	}
}
