package promptguard

import "testing"

func TestScoreAnalysisAccumulatesWeightsAndCategories(t *testing.T) {
	score, categories := scoreAnalysis([]RuleMatch{
		{RuleID: "a", Category: RuleCategoryOverrideAttempt, Weight: 2},
		{RuleID: "b", Category: RuleCategoryPromptExfiltration, Weight: 8},
		{RuleID: "c", Category: RuleCategoryPromptExfiltration, Weight: 2},
	})

	if score != 12 {
		t.Fatalf("expected score 12, got %d", score)
	}
	if len(categories) != 2 {
		t.Fatalf("expected 2 deduped categories, got %+v", categories)
	}
}
