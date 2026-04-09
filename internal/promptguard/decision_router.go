package promptguard

import "strings"

const blockThreshold = 8

func routeDecision(score int, matches []RuleMatch, categories []RuleCategory) (Decision, string) {
	if len(matches) == 0 || score <= 0 {
		return DecisionAllow, textRuleAllowReason
	}
	if score >= blockThreshold {
		return DecisionBlock, buildCategoryReason(textRuleBlockBaseReason, categories)
	}
	return DecisionNeedsLLM, buildCategoryReason(textRuleNeedsLLMReason, categories)
}

func buildCategoryReason(prefix string, categories []RuleCategory) string {
	labels := make([]string, 0, len(categories))
	for _, category := range categories {
		if label := categoryLabel(category); label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return prefix
	}
	return prefix + "：" + strings.Join(labels, "、")
}

func categoryLabel(category RuleCategory) string {
	switch category {
	case RuleCategoryOverrideAttempt:
		return "覆寫規則"
	case RuleCategoryPromptExfiltration:
		return "索取提示詞"
	case RuleCategoryRoleSpoofing:
		return "角色偽裝"
	case RuleCategorySafetyBypass:
		return "繞過安全限制"
	default:
		return ""
	}
}
