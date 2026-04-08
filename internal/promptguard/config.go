package promptguard

import (
	"os"
	"strings"
)

const (
	envPromptGuardMode    = "INTERNAL_AI_COPILOT_PROMPTGUARD_MODE"
	envPromptGuardModel   = "INTERNAL_AI_COPILOT_PROMPTGUARD_MODEL"
	envPromptGuardBaseURL = "INTERNAL_AI_COPILOT_PROMPTGUARD_BASE_URL"
	envPromptGuardAPIKey  = "INTERNAL_AI_COPILOT_PROMPTGUARD_API_KEY"

	defaultPromptGuardCloudBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultPromptGuardModel        = "gemma-4-31b-it"
)

// Config keeps promptguard runtime settings isolated from the main AI client.
type Config struct {
	Mode    Mode
	Model   string
	BaseURL string
	APIKey  string
}

// LoadConfigFromEnv reads promptguard runtime config from its dedicated env keys.
func LoadConfigFromEnv() Config {
	return Config{
		Mode:    getenvMode(envPromptGuardMode, ModeCloud),
		Model: firstNonEmptyEnv(
			envPromptGuardModel,
			"INTERNAL_AI_COPILOT_GEMMA_MODEL",
			"REWARDBRIDGE_GEMMA_MODEL",
		),
		BaseURL: firstNonEmptyEnv(
			envPromptGuardBaseURL,
			"INTERNAL_AI_COPILOT_GEMMA_BASE_URL",
			"REWARDBRIDGE_GEMMA_BASE_URL",
		),
		APIKey: firstNonEmptyEnv(
			envPromptGuardAPIKey,
			"INTERNAL_AI_COPILOT_GEMMA_API_KEY",
			"REWARDBRIDGE_GEMMA_API_KEY",
			"GEMINI_API_KEY",
			"GOOGLE_API_KEY",
		),
	}
}

func (c Config) resolvedMode() Mode {
	if parsed, ok := ParseMode(string(c.Mode)); ok {
		return parsed
	}
	return ModeCloud
}

func (c Config) resolvedModel() string {
	if value := strings.TrimSpace(c.Model); value != "" {
		return value
	}
	return defaultPromptGuardModel
}

func (c Config) resolvedBaseURL() string {
	if value := strings.TrimSpace(c.BaseURL); value != "" {
		return value
	}
	if c.resolvedMode() == ModeCloud {
		return defaultPromptGuardCloudBaseURL
	}
	return ""
}

func getenvMode(key string, fallback Mode) Mode {
	if parsed, ok := ParseMode(os.Getenv(key)); ok {
		return parsed
	}
	return fallback
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
