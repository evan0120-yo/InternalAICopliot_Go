package promptguard

import (
	"strings"
)

func matchFeatures(normalizedText string, catalog []Rule) []RuleMatch {
	if normalizedText == "" || len(catalog) == 0 {
		return nil
	}

	matches := make([]RuleMatch, 0, len(catalog))
	for _, rule := range catalog {
		match, ok := matchRule(normalizedText, rule)
		if ok {
			matches = append(matches, match)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func matchRule(normalizedText string, rule Rule) (RuleMatch, bool) {
	if !rule.Enabled {
		return RuleMatch{}, false
	}

	switch rule.MatchType {
	case MatchTypeKeyword, MatchTypePhrase:
		if rule.normalizedPattern == "" || !strings.Contains(normalizedText, rule.normalizedPattern) {
			return RuleMatch{}, false
		}
		return RuleMatch{
			RuleID:    rule.ID,
			Category:  rule.Category,
			MatchType: rule.MatchType,
			Weight:    rule.Weight,
			Evidence:  rule.normalizedPattern,
		}, true
	case MatchTypeRegex:
		if rule.compiledRegex == nil {
			return RuleMatch{}, false
		}
		evidence := rule.compiledRegex.FindString(normalizedText)
		if evidence == "" {
			return RuleMatch{}, false
		}
		return RuleMatch{
			RuleID:    rule.ID,
			Category:  rule.Category,
			MatchType: rule.MatchType,
			Weight:    rule.Weight,
			Evidence:  evidence,
		}, true
	case MatchTypeCombo:
		if len(rule.normalizedTerms) == 0 {
			return RuleMatch{}, false
		}
		evidenceTerms := make([]string, 0, len(rule.normalizedTerms))
		for _, normalizedTerm := range rule.normalizedTerms {
			if !strings.Contains(normalizedText, normalizedTerm) {
				return RuleMatch{}, false
			}
			evidenceTerms = append(evidenceTerms, normalizedTerm)
		}
		return RuleMatch{
			RuleID:    rule.ID,
			Category:  rule.Category,
			MatchType: rule.MatchType,
			Weight:    rule.Weight,
			Evidence:  strings.Join(evidenceTerms, " + "),
		}, true
	default:
		return RuleMatch{}, false
	}
}
