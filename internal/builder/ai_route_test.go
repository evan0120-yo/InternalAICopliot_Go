package builder

import (
	"testing"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
)

func TestChooseAIRouteCodeUsesGPT54ForProfileConsult(t *testing.T) {
	route := chooseAIRouteCode(
		ConsultCommand{Mode: ConsultModeProfile},
		infra.BuilderConfig{BuilderCode: "linkchat-astrology"},
		aiclient.AIRouteDirectGemma,
	)

	if route != aiclient.AIRouteDirectGPT54 {
		t.Fatalf("expected profile consult to force direct_gpt54, got %s", route)
	}
}

func TestChooseAIRouteCodeUsesGemmaForLineMemoBuilders(t *testing.T) {
	route := chooseAIRouteCode(
		ConsultCommand{Mode: ConsultModeExtract},
		infra.BuilderConfig{BuilderCode: "line-memo-crud"},
		aiclient.AIRouteDirectGPT54,
	)

	if route != aiclient.AIRouteDirectGemma {
		t.Fatalf("expected line memo builder to use direct_gemma, got %s", route)
	}
}

func TestChooseAIRouteCodeFallsBackToDefaultRoute(t *testing.T) {
	route := chooseAIRouteCode(
		ConsultCommand{Mode: ConsultModeGeneric},
		infra.BuilderConfig{BuilderCode: "pm-estimate"},
		aiclient.AIRouteGemmaThenGPT54,
	)

	if route != aiclient.AIRouteGemmaThenGPT54 {
		t.Fatalf("expected default route fallback, got %s", route)
	}
}
