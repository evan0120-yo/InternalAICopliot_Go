package promptguard

import (
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

// Mode selects which LLM guard endpoint should be used.
type Mode string

const (
	ModeCloud Mode = "cloud"
	ModeLocal Mode = "local"
)

const (
	needsLLMPlaceholderScore       = 50
	textRulePlaceholderReason      = "TEXT_RULE_PLACEHOLDER"
	llmGuardCloudPlaceholderReason = "LLM_GUARD_CLOUD_PLACEHOLDER"
	llmGuardLocalPlaceholderReason = "LLM_GUARD_LOCAL_PLACEHOLDER"
)

// Command is the promptguard entry contract from gatekeeper.
type Command struct {
	AppID         string
	BuilderConfig infra.BuilderConfig
	RawUserText   string
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

// Evaluation is the unified promptguard result contract.
type Evaluation struct {
	Decision Decision `json:"decision"`
	Score    int      `json:"score"`
	Reason   string   `json:"reason"`
	Source   Source   `json:"source"`
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
