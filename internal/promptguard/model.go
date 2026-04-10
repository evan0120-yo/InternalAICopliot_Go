package promptguard

import (
	"regexp"
	"strings"

	"com.citrus.internalaicopilot/internal/infra"
)

// Decision is the normalized guard outcome.
type Decision string

const (
	DecisionAllow    Decision = "allow"
	DecisionBlock    Decision = "block"
	DecisionNeedsLLM Decision = "needs_llm"
)

// Source is the evaluator that produced the decision.
type Source string

const (
	SourceTextRule Source = "text_rule"
	SourceLLMGuard Source = "llm_guard"
)

// MatchType defines how one rule should be matched against text.
type MatchType string

const (
	MatchTypeKeyword MatchType = "keyword"
	MatchTypePhrase  MatchType = "phrase"
	MatchTypeRegex   MatchType = "regex"
	MatchTypeCombo   MatchType = "combo"
)

// RuleCategory classifies one risk family.
type RuleCategory string

const (
	RuleCategoryOverrideAttempt    RuleCategory = "override_attempt"
	RuleCategoryPromptExfiltration RuleCategory = "prompt_exfiltration"
	RuleCategoryRoleSpoofing       RuleCategory = "role_spoofing"
	RuleCategorySafetyBypass       RuleCategory = "safety_bypass"
)

// Mode selects which LLM guard endpoint should be used.
type Mode string

const (
	ModeCloud Mode = "cloud"
	ModeLocal Mode = "local"
)

const (
	needsLLMPlaceholderScore       = 50
	llmGuardCloudPlaceholderReason = "LLM_GUARD_CLOUD_PLACEHOLDER"
	llmGuardLocalPlaceholderReason = "LLM_GUARD_LOCAL_PLACEHOLDER"
)

const (
	textRuleAllowReason     = "未命中文本風險規則"
	textRuleNeedsLLMReason  = "命中灰區風險規則"
	textRuleBlockBaseReason = "命中高風險規則"
)

// Command is the promptguard entry contract from gatekeeper.
type Command struct {
	AppID         string
	BuilderConfig infra.BuilderConfig
	UserText      string
}

// GuardPrompt is the builder-owned dedicated guard prompt payload.
type GuardPrompt struct {
	Instructions    string
	UserMessageText string
}

// GuardLLMRequest is the aiclient-facing second-layer guard request.
type GuardLLMRequest struct {
	Mode            Mode
	Model           string
	BaseURL         string
	APIKey          string
	Instructions    string
	UserMessageText string
}

// GuardLLMResponse is the parsed dedicated guard JSON contract.
type GuardLLMResponse struct {
	Status    bool   `json:"status"`
	StatusAns string `json:"statusAns"`
	Reason    string `json:"reason"`
}

// Rule is one static rule-catalog definition.
type Rule struct {
	ID        string
	Category  RuleCategory
	MatchType MatchType
	Weight    int
	Enabled   bool
	Pattern   string
	Terms     []string

	// Precomputed matching data keeps static catalog work out of the request hot path.
	normalizedPattern string
	normalizedTerms   []string
	compiledRegex     *regexp.Regexp
}

// RuleMatch is one dynamic evidence hit for one request.
type RuleMatch struct {
	RuleID    string       `json:"ruleId"`
	Category  RuleCategory `json:"category"`
	MatchType MatchType    `json:"matchType"`
	Weight    int          `json:"weight"`
	Evidence  string       `json:"evidence"`
}

// TextAnalysis is the internal first-layer pipeline result.
type TextAnalysis struct {
	RawText           string
	NormalizedText    string
	Matches           []RuleMatch
	MatchedCategories []RuleCategory
	Score             int
	Decision          Decision
	Reason            string
}

// Evaluation is the unified promptguard result contract.
type Evaluation struct {
	Decision          Decision       `json:"decision"`
	Score             int            `json:"score"`
	Reason            string         `json:"reason"`
	Source            Source         `json:"source"`
	MatchedRules      []string       `json:"matchedRules,omitempty"`
	MatchedCategories []RuleCategory `json:"matchedCategories,omitempty"`
}

// ParseMode validates configured promptguard mode input.
func ParseMode(raw string) (Mode, bool) {
	switch Mode(strings.ToLower(strings.TrimSpace(raw))) {
	case ModeCloud:
		return ModeCloud, true
	case ModeLocal:
		return ModeLocal, true
	default:
		return "", false
	}
}

func (analysis TextAnalysis) Evaluation() Evaluation {
	return Evaluation{
		Decision:          analysis.Decision,
		Score:             analysis.Score,
		Reason:            analysis.Reason,
		Source:            SourceTextRule,
		MatchedRules:      matchedRuleIDs(analysis.Matches),
		MatchedCategories: append([]RuleCategory(nil), analysis.MatchedCategories...),
	}
}

func matchedRuleIDs(matches []RuleMatch) []string {
	if len(matches) == 0 {
		return nil
	}

	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if value := strings.TrimSpace(match.RuleID); value != "" {
			ids = append(ids, value)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}
