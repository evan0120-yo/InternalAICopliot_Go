package builder

import (
	"regexp"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

const CommonModuleKey = "common"

var moduleKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func canonicalModuleKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NormalizeStoredModuleKey validates source module keys and folds common into empty.
func NormalizeStoredModuleKey(raw string) (string, error) {
	moduleKey := canonicalModuleKey(raw)
	if moduleKey == "" || moduleKey == CommonModuleKey {
		return "", nil
	}
	if !moduleKeyPattern.MatchString(moduleKey) {
		return "", infra.NewError("INVALID_MODULE_KEY", "source moduleKey must match ^[a-z0-9][a-z0-9_-]*$.", 400)
	}
	return moduleKey, nil
}

// NormalizeAnalysisTypeKey trims and validates a single analysis type key.
func NormalizeAnalysisTypeKey(raw string) (string, error) {
	return normalizeRequestedModuleKey(raw, "subjectProfile.analysisPayloads.analysisType")
}

func normalizeRequestedModuleKey(raw, fieldLabel string) (string, error) {
	moduleKey := canonicalModuleKey(raw)
	if moduleKey == "" {
		return "", infra.NewError("INVALID_MODULE_KEY", fieldLabel+" must not be blank.", 400)
	}
	if moduleKey == CommonModuleKey {
		return "", infra.NewError("RESERVED_MODULE_KEY", fieldLabel+" must not include the reserved common module key.", 400)
	}
	if !moduleKeyPattern.MatchString(moduleKey) {
		return "", infra.NewError("INVALID_MODULE_KEY", fieldLabel+" must match ^[a-z0-9][a-z0-9_-]*$.", 400)
	}
	return moduleKey, nil
}
