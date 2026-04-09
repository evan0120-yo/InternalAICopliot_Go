package promptguard

import "testing"

func TestRouteDecisionReturnsAllowWithoutMatches(t *testing.T) {
	decision, reason := routeDecision(0, nil, nil)
	if decision != DecisionAllow || reason != textRuleAllowReason {
		t.Fatalf("unexpected allow route: decision=%s reason=%s", decision, reason)
	}
}

func TestRouteDecisionReturnsNeedsLLMForMidRisk(t *testing.T) {
	decision, reason := routeDecision(2, []RuleMatch{{RuleID: "keyword.system_prompt"}}, []RuleCategory{RuleCategoryPromptExfiltration})
	if decision != DecisionNeedsLLM {
		t.Fatalf("expected needs_llm, got %s", decision)
	}
	if reason != "命中灰區風險規則：索取提示詞" {
		t.Fatalf("unexpected needs_llm reason: %s", reason)
	}
}

func TestRouteDecisionReturnsBlockForHighRisk(t *testing.T) {
	decision, reason := routeDecision(8, []RuleMatch{{RuleID: "override.ignore_previous_instructions"}}, []RuleCategory{RuleCategoryOverrideAttempt})
	if decision != DecisionBlock {
		t.Fatalf("expected block, got %s", decision)
	}
	if reason != "命中高風險規則：覆寫規則" {
		t.Fatalf("unexpected block reason: %s", reason)
	}
}
