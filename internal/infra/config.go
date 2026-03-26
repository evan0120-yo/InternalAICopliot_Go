package infra

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config keeps runtime settings intentionally small and env-driven.
type Config struct {
	Addr                  string
	GRPCAddr              string
	FirestoreProjectID    string
	FirestoreEmulatorHost string
	StoreResetOnStart     bool
	CORSAllowedOrigins    []string
	ConsultMaxFiles       int
	ConsultMaxFileSize    int64
	ConsultMaxTotalSize   int64
	ServerReadTimeout     time.Duration
	ServerWriteTimeout    time.Duration
	OpenAITimeout         time.Duration
	AIPreviewMode         bool
	OpenAIAPIKey          string
	OpenAIBaseURL         string
	OpenAIModel           string
}

// LoadConfigFromEnv reads runtime config with dev-safe defaults.
func LoadConfigFromEnv() Config {
	return Config{
		Addr:                  getenvCompat("INTERNAL_AI_COPILOT_ADDR", "REWARDBRIDGE_ADDR", ":8082"),
		GRPCAddr:              getenvCompat("INTERNAL_AI_COPILOT_GRPC_ADDR", "REWARDBRIDGE_GRPC_ADDR", ":9091"),
		FirestoreProjectID:    getenvCompat("INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID", "REWARDBRIDGE_FIRESTORE_PROJECT_ID", getenv("GCLOUD_PROJECT", getenv("GOOGLE_CLOUD_PROJECT", "dailo-467502"))),
		FirestoreEmulatorHost: getenvCompat("INTERNAL_AI_COPILOT_FIRESTORE_EMULATOR_HOST", "REWARDBRIDGE_FIRESTORE_EMULATOR_HOST", getenv("FIRESTORE_EMULATOR_HOST", "localhost:8090")),
		StoreResetOnStart:     getenvBoolCompat("INTERNAL_AI_COPILOT_STORE_RESET_ON_START", "REWARDBRIDGE_STORE_RESET_ON_START", false),
		CORSAllowedOrigins:    getenvCSVCompat("INTERNAL_AI_COPILOT_CORS_ALLOWED_ORIGINS", "REWARDBRIDGE_CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://127.0.0.1:3000"}),
		ConsultMaxFiles:       getenvIntCompat("INTERNAL_AI_COPILOT_CONSULT_MAX_FILES", "REWARDBRIDGE_CONSULT_MAX_FILES", 10),
		ConsultMaxFileSize:    getenvInt64Compat("INTERNAL_AI_COPILOT_CONSULT_MAX_FILE_SIZE_BYTES", "REWARDBRIDGE_CONSULT_MAX_FILE_SIZE_BYTES", 20*1024*1024),
		ConsultMaxTotalSize:   getenvInt64Compat("INTERNAL_AI_COPILOT_CONSULT_MAX_TOTAL_SIZE_BYTES", "REWARDBRIDGE_CONSULT_MAX_TOTAL_SIZE_BYTES", 50*1024*1024),
		ServerReadTimeout:     getenvDurationCompat("INTERNAL_AI_COPILOT_SERVER_READ_TIMEOUT", "REWARDBRIDGE_SERVER_READ_TIMEOUT", 10*time.Second),
		ServerWriteTimeout:    getenvDurationCompat("INTERNAL_AI_COPILOT_SERVER_WRITE_TIMEOUT", "REWARDBRIDGE_SERVER_WRITE_TIMEOUT", 180*time.Second),
		OpenAITimeout:         getenvDurationCompat("INTERNAL_AI_COPILOT_OPENAI_TIMEOUT", "REWARDBRIDGE_OPENAI_TIMEOUT", 120*time.Second),
		AIPreviewMode:         getenvBoolCompat("INTERNAL_AI_COPILOT_AI_PREVIEW_MODE", "REWARDBRIDGE_AI_PREVIEW_MODE", false),
		OpenAIAPIKey:          os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:         getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:           getenvCompat("INTERNAL_AI_COPILOT_AI_MODEL", "REWARDBRIDGE_AI_MODEL", "gpt-4o"),
	}
}

func getenvCompat(primaryKey, legacyKey, fallback string) string {
	if value := os.Getenv(primaryKey); value != "" {
		return value
	}
	return getenv(legacyKey, fallback)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvIntCompat(primaryKey, legacyKey string, fallback int) int {
	if value := os.Getenv(primaryKey); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return getenvInt(legacyKey, fallback)
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvInt64Compat(primaryKey, legacyKey string, fallback int64) int64 {
	if value := os.Getenv(primaryKey); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return getenvInt64(legacyKey, fallback)
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBoolCompat(primaryKey, legacyKey string, fallback bool) bool {
	if value := os.Getenv(primaryKey); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return getenvBool(legacyKey, fallback)
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDurationCompat(primaryKey, legacyKey string, fallback time.Duration) time.Duration {
	if value := os.Getenv(primaryKey); value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}
	return getenvDuration(legacyKey, fallback)
}

func getenvCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func getenvCSVCompat(primaryKey, legacyKey string, fallback []string) []string {
	if value := os.Getenv(primaryKey); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return getenvCSV(legacyKey, fallback)
}
