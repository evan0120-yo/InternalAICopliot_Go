package infra

import (
	"os"
	"strconv"
	"strings"
)

const (
	// EnvAIProfile is the compact runtime selector for main AI + promptguard behavior.
	EnvAIProfile = "INTERNAL_AI_COPILOT_AI_PROFILE"

	DefaultOpenAIBaseURL     = "https://api.openai.com/v1"
	DefaultGemmaBaseURL      = "https://generativelanguage.googleapis.com/v1beta"
	DefaultOpenAIModel       = "gpt-4o"
	DefaultGemmaModel        = "gemma-4-31b-it"
	DefaultLocalGemmaBaseURL = "http://localhost:11434"
)

// AIRuntimeProfile is the compact runtime scenario selected by AI_PROFILE.
type AIRuntimeProfile struct {
	ID                 int
	Name               string
	MainMode           AIExecutionMode
	MainMock           bool
	MainProvider       AIProvider
	MainModel          string
	MainBaseURL        string
	PromptGuardMode    string
	PromptGuardModel   string
	PromptGuardBaseURL string
}

// ParseAIRuntimeProfile resolves a numeric profile id into a full runtime scenario.
func ParseAIRuntimeProfile(raw string) (AIRuntimeProfile, bool) {
	profileID, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return AIRuntimeProfile{}, false
	}
	return ResolveAIRuntimeProfile(profileID)
}

// ResolveAIRuntimeProfile resolves a profile id into runtime behavior.
func ResolveAIRuntimeProfile(id int) (AIRuntimeProfile, bool) {
	switch id {
	case 1:
		return AIRuntimeProfile{
			ID:                 1,
			Name:               "preview_full + promptguard_cloud + main_openai",
			MainMode:           AIExecutionModePreviewFull,
			MainMock:           false,
			MainProvider:       AIProviderOpenAI,
			MainModel:          DefaultOpenAIModel,
			MainBaseURL:        DefaultOpenAIBaseURL,
			PromptGuardMode:    "cloud",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultGemmaBaseURL,
		}, true
	case 2:
		return AIRuntimeProfile{
			ID:                 2,
			Name:               "preview_prompt_body_only + promptguard_cloud + main_openai",
			MainMode:           AIExecutionModePreviewPromptBodyOnly,
			MainMock:           false,
			MainProvider:       AIProviderOpenAI,
			MainModel:          DefaultOpenAIModel,
			MainBaseURL:        DefaultOpenAIBaseURL,
			PromptGuardMode:    "cloud",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultGemmaBaseURL,
		}, true
	case 3:
		return AIRuntimeProfile{
			ID:                 3,
			Name:               "live_mock + promptguard_cloud",
			MainMode:           AIExecutionModeLive,
			MainMock:           true,
			MainProvider:       AIProviderOpenAI,
			MainModel:          DefaultOpenAIModel,
			MainBaseURL:        DefaultOpenAIBaseURL,
			PromptGuardMode:    "cloud",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultGemmaBaseURL,
		}, true
	case 4:
		return AIRuntimeProfile{
			ID:                 4,
			Name:               "live_openai + promptguard_cloud",
			MainMode:           AIExecutionModeLive,
			MainMock:           false,
			MainProvider:       AIProviderOpenAI,
			MainModel:          DefaultOpenAIModel,
			MainBaseURL:        DefaultOpenAIBaseURL,
			PromptGuardMode:    "cloud",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultGemmaBaseURL,
		}, true
	case 5:
		return AIRuntimeProfile{
			ID:                 5,
			Name:               "live_gemma + promptguard_cloud",
			MainMode:           AIExecutionModeLive,
			MainMock:           false,
			MainProvider:       AIProviderGemma,
			MainModel:          DefaultGemmaModel,
			MainBaseURL:        DefaultGemmaBaseURL,
			PromptGuardMode:    "cloud",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultGemmaBaseURL,
		}, true
	case 6:
		return AIRuntimeProfile{
			ID:                 6,
			Name:               "live_openai + promptguard_local",
			MainMode:           AIExecutionModeLive,
			MainMock:           false,
			MainProvider:       AIProviderOpenAI,
			MainModel:          DefaultOpenAIModel,
			MainBaseURL:        DefaultOpenAIBaseURL,
			PromptGuardMode:    "local",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultLocalGemmaBaseURL,
		}, true
	case 7:
		return AIRuntimeProfile{
			ID:                 7,
			Name:               "live_gemma + promptguard_local",
			MainMode:           AIExecutionModeLive,
			MainMock:           false,
			MainProvider:       AIProviderGemma,
			MainModel:          DefaultGemmaModel,
			MainBaseURL:        DefaultGemmaBaseURL,
			PromptGuardMode:    "local",
			PromptGuardModel:   DefaultGemmaModel,
			PromptGuardBaseURL: DefaultLocalGemmaBaseURL,
		}, true
	default:
		return AIRuntimeProfile{}, false
	}
}

// LoadAIRuntimeProfileFromEnv reads AI_PROFILE from env and resolves it.
func LoadAIRuntimeProfileFromEnv() (AIRuntimeProfile, bool) {
	if profile, ok := ParseAIRuntimeProfile(os.Getenv(EnvAIProfile)); ok {
		return profile, true
	}
	return AIRuntimeProfile{}, false
}
