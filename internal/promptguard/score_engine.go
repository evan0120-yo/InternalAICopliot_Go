package promptguard

func scoreAnalysis(matches []RuleMatch) (int, []RuleCategory) {
	if len(matches) == 0 {
		return 0, nil
	}

	score := 0
	categories := make([]RuleCategory, 0, len(matches))
	seenCategories := make(map[RuleCategory]struct{}, len(matches))
	for _, match := range matches {
		score += match.Weight
		if _, seen := seenCategories[match.Category]; !seen && match.Category != "" {
			seenCategories[match.Category] = struct{}{}
			categories = append(categories, match.Category)
		}
	}
	return score, categories
}
