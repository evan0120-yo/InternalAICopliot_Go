package builder

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
)

func TestConsultTaskBuilderFactorySelectsBuilderByMode(t *testing.T) {
	factory := newConsultTaskBuilderFactory(aiclient.AIRouteDirectGPT54)

	cases := []struct {
		name     string
		command  ConsultCommand
		expected any
	}{
		{name: "generic", command: ConsultCommand{Mode: ConsultModeGeneric}, expected: genericConsultTaskBuilder{}},
		{name: "profile", command: ConsultCommand{Mode: ConsultModeProfile}, expected: profileConsultTaskBuilder{}},
		{name: "extract", command: ConsultCommand{Mode: ConsultModeExtract}, expected: extractConsultTaskBuilder{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder := factory.BuilderFor(tc.command)
			if reflect.TypeOf(builder) != reflect.TypeOf(tc.expected) {
				t.Fatalf("expected %T, got %T", tc.expected, builder)
			}
		})
	}
}

func TestExtractConsultTaskBuilderBuildReturnsExtractionPlan(t *testing.T) {
	builder := extractConsultTaskBuilder{defaultAIRoute: aiclient.AIRouteDirectGPT54}
	assembleService := NewAssembleService(nil)

	result, err := builder.Build(context.Background(), assembleService, consultTaskBuildInput{
		Command: ConsultCommand{
			Mode:          ConsultModeExtract,
			Text:          "小傑 明天 下午三點找我吃飯",
			ReferenceTime: "2026-04-14 10:00:00",
			TimeZone:      "Asia/Taipei",
		},
		BuilderConfig: infra.BuilderConfig{
			BuilderCode: "line-memo-crud",
			Name:        "Line Memo CRUD",
		},
		Sources: []infra.Source{
			{SourceID: 1, OrderNo: 1, Prompts: "補充 extraction 指示"},
		},
		RagsBySourceID: map[int64][]infra.RagSupplement{},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Route != aiclient.AIRouteDirectGemma {
		t.Fatalf("expected direct_gemma route, got %s", result.Route)
	}
	if result.ResponseContract != aiclient.AnalyzeResponseContractExtraction {
		t.Fatalf("expected extraction contract, got %s", result.ResponseContract)
	}
	if !strings.Contains(result.Prompt.Instructions, "## [REFERENCE_TIME]") {
		t.Fatalf("expected extraction prompt sections, got %s", result.Prompt.Instructions)
	}
}
