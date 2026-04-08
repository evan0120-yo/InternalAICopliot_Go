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
		Model:   strings.TrimSpace(os.Getenv(envPromptGuardModel)),
		BaseURL: strings.TrimSpace(os.Getenv(envPromptGuardBaseURL)),
		APIKey:  strings.TrimSpace(os.Getenv(envPromptGuardAPIKey)),
	}
}

func (c Config) resolvedMode() Mode {
	if parsed, ok := ParseMode(string(c.Mode)); ok {
		return parsed
	}
	return ModeCloud
}

func getenvMode(key string, fallback Mode) Mode {
	if parsed, ok := ParseMode(os.Getenv(key)); ok {
		return parsed
	}
	return fallback
}
