package builder

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

type weightedPayloadEntry struct {
	Key           string
	WeightPercent *int
}

// ValidateWeightedPayloadEnvelope validates the shared weighted-entry shape without applying domain-specific semantics.
func ValidateWeightedPayloadEnvelope(rootPath string, payload map[string]any) error {
	if len(payload) == 0 {
		return nil
	}

	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := strings.TrimSpace(rootPath)
		if path == "" {
			path = key
		} else {
			path = path + "." + key
		}
		if err := validateWeightedPayloadValue(path, payload[key]); err != nil {
			return err
		}
	}
	return nil
}

func flattenPayloadToWeightedEntryMap(payload map[string]any) (map[string][]weightedPayloadEntry, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	flattened := make(map[string][]weightedPayloadEntry)
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := flattenWeightedPayloadValue(canonicalPayloadKey(key), payload[key], flattened); err != nil {
			return nil, err
		}
	}
	return flattened, nil
}

func flattenWeightedPayloadValue(prefix string, value any, flattened map[string][]weightedPayloadEntry) error {
	if prefix == "" {
		return nil
	}

	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed != "" {
			flattened[prefix] = append(flattened[prefix], weightedPayloadEntry{Key: trimmed})
		}
		return nil
	case bool, float64, int, int32, int64:
		flattened[prefix] = append(flattened[prefix], weightedPayloadEntry{Key: stringifyPayloadScalar(typed)})
		return nil
	case []string:
		for _, item := range typed {
			if err := flattenWeightedPayloadValue(prefix, item, flattened); err != nil {
				return err
			}
		}
		return nil
	case []any:
		entries, isWeightedArray, err := extractWeightedEntries(typed)
		if err != nil {
			return err
		}
		if isWeightedArray {
			flattened[prefix] = append(flattened[prefix], entries...)
			return nil
		}
		for _, item := range typed {
			if err := flattenWeightedPayloadValue(prefix, item, flattened); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		entry, isWeightedEntry, err := extractWeightedEntry(typed)
		if err != nil {
			return err
		}
		if isWeightedEntry {
			flattened[prefix] = append(flattened[prefix], entry)
			return nil
		}
		childKeys := make([]string, 0, len(typed))
		for key := range typed {
			childKeys = append(childKeys, key)
		}
		sort.Strings(childKeys)
		for _, key := range childKeys {
			childPrefix := canonicalPayloadKey(prefix + "_" + key)
			if err := flattenWeightedPayloadValue(childPrefix, typed[key], flattened); err != nil {
				return err
			}
		}
		return nil
	default:
		return infra.NewError("INVALID_ANALYSIS_PAYLOAD", "analysis payload contains an unsupported weighted value.", http.StatusBadRequest)
	}
}

func validateWeightedPayloadValue(path string, value any) error {
	switch typed := value.(type) {
	case nil, string, bool, float64, int, int32, int64:
		return nil
	case []string:
		return nil
	case []any:
		_, isWeightedArray, err := extractWeightedEntries(typed)
		if err != nil {
			return wrapWeightedPayloadError(path, err)
		}
		if isWeightedArray {
			return nil
		}
		for index, item := range typed {
			if err := validateWeightedPayloadValue(fmt.Sprintf("%s[%d]", path, index), item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		_, isWeightedEntry, err := extractWeightedEntry(typed)
		if err != nil {
			return wrapWeightedPayloadError(path, err)
		}
		if isWeightedEntry {
			return nil
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateWeightedPayloadValue(path+"."+key, typed[key]); err != nil {
				return err
			}
		}
		return nil
	default:
		return infra.NewError("INVALID_ANALYSIS_PAYLOAD", path+" contains an unsupported analysis payload value.", http.StatusBadRequest)
	}
}

func extractWeightedEntries(values []any) ([]weightedPayloadEntry, bool, error) {
	if len(values) == 0 {
		return nil, false, nil
	}

	hasWeightedCandidate := false
	for _, item := range values {
		typed, ok := item.(map[string]any)
		if ok && isWeightedEntryCandidate(typed) {
			hasWeightedCandidate = true
			break
		}
	}
	if !hasWeightedCandidate {
		return nil, false, nil
	}

	entries := make([]weightedPayloadEntry, 0, len(values))
	totalWeight := 0
	for index, item := range values {
		typed, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("[%d] must be an object when using weighted entries", index)
		}
		entry, isWeightedEntry, err := extractWeightedEntry(typed)
		if err != nil {
			return nil, true, fmt.Errorf("[%d] %w", index, err)
		}
		if !isWeightedEntry {
			return nil, true, fmt.Errorf("[%d] must contain key when using weighted entries", index)
		}
		if len(values) > 1 && entry.WeightPercent == nil {
			return nil, true, fmt.Errorf("[%d].weightPercent is required when multiple entries are provided", index)
		}
		if entry.WeightPercent != nil {
			totalWeight += *entry.WeightPercent
		}
		entries = append(entries, entry)
	}
	if len(values) > 1 && totalWeight != 100 {
		return nil, true, fmt.Errorf("weightPercent total must equal 100")
	}
	return entries, true, nil
}

func extractWeightedEntry(value map[string]any) (weightedPayloadEntry, bool, error) {
	if !isWeightedEntryCandidate(value) {
		return weightedPayloadEntry{}, false, nil
	}

	rawKey, ok := value["key"]
	if !ok {
		return weightedPayloadEntry{}, true, fmt.Errorf(".key is required")
	}
	key := strings.TrimSpace(stringifyPayloadScalar(rawKey))
	if key == "" {
		return weightedPayloadEntry{}, true, fmt.Errorf(".key must not be blank")
	}

	weightPercent, err := extractWeightPercent(value)
	if err != nil {
		return weightedPayloadEntry{}, true, err
	}

	return weightedPayloadEntry{
		Key:           key,
		WeightPercent: weightPercent,
	}, true, nil
}

func extractWeightPercent(value map[string]any) (*int, error) {
	rawWeight, ok := value["weightPercent"]
	if !ok {
		return nil, nil
	}

	switch typed := rawWeight.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return nil, fmt.Errorf(".weightPercent must be an integer")
		}
		parsed := int(typed)
		if parsed < 0 || parsed > 100 {
			return nil, fmt.Errorf(".weightPercent must be between 0 and 100")
		}
		return &parsed, nil
	case int:
		if typed < 0 || typed > 100 {
			return nil, fmt.Errorf(".weightPercent must be between 0 and 100")
		}
		parsed := typed
		return &parsed, nil
	case int32:
		parsed := int(typed)
		if parsed < 0 || parsed > 100 {
			return nil, fmt.Errorf(".weightPercent must be between 0 and 100")
		}
		return &parsed, nil
	case int64:
		parsed := int(typed)
		if parsed < 0 || parsed > 100 {
			return nil, fmt.Errorf(".weightPercent must be between 0 and 100")
		}
		return &parsed, nil
	default:
		return nil, fmt.Errorf(".weightPercent must be numeric")
	}
}

func isWeightedEntryCandidate(value map[string]any) bool {
	_, hasKey := value["key"]
	_, hasWeightPercent := value["weightPercent"]
	return hasKey || hasWeightPercent
}

func wrapWeightedPayloadError(path string, err error) error {
	return infra.NewError("INVALID_ANALYSIS_PAYLOAD", path+" "+err.Error()+".", http.StatusBadRequest)
}
