package grpcapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/gatekeeper"
	grpcpb "com.citrus.internalaicopilot/internal/grpcapi/pb"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"
)

func TestToSubjectProfilePreservesTheoryVersion(t *testing.T) {
	theoryVersion := "astro-v1"
	profile := toSubjectProfile(&grpcpb.SubjectProfile{
		SubjectId: "user-123",
		ModulePayloads: []*grpcpb.SubjectModulePayload{
			{
				ModuleKey:     "astrology",
				TheoryVersion: &theoryVersion,
				Facts: []*grpcpb.SubjectFact{
					{FactKey: "sun_sign", Values: []string{"Scorpio"}},
				},
			},
		},
	})

	if profile == nil {
		t.Fatalf("expected profile to be converted")
	}
	if profile.ModulePayloads[0].TheoryVersion == nil || *profile.ModulePayloads[0].TheoryVersion != "astro-v1" {
		t.Fatalf("expected theory version to be preserved, got %+v", profile.ModulePayloads[0].TheoryVersion)
	}
}

func TestProfileConsultPassesTheoryMappedPayloadThroughService(t *testing.T) {
	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:     fmt.Sprintf("grpcapi-service-%d", time.Now().UnixNano()),
		ResetOnStart:  true,
		SeedWhenEmpty: true,
		SeedData:      infra.DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := infra.Config{
		ConsultMaxFiles:     10,
		ConsultMaxFileSize:  20 * 1024 * 1024,
		ConsultMaxTotalSize: 50 * 1024 * 1024,
	}
	builderQuery := builder.NewQueryService(store)
	ragUseCase := rag.NewResolveUseCase(rag.NewResolveService(store))
	aiUseCase := aiclient.NewAnalyzeUseCase(aiclient.NewAnalyzeService(cfg))
	outputUseCase := output.NewRenderUseCase(output.NewRenderService())
	builderConsult := builder.NewConsultUseCase(store, ragUseCase, aiUseCase, outputUseCase, builder.NewAssembleService(store), "gpt-4o")
	useCase := gatekeeper.NewUseCase(gatekeeper.NewGuardService(cfg, store), builderQuery, builderConsult)
	service := New(useCase)

	theoryVersion := "astro-v1"
	response, err := service.ProfileConsult(context.Background(), &grpcpb.ProfileConsultRequest{
		AppId:           "linkchat",
		BuilderId:       1,
		AnalysisModules: []string{"astrology"},
		SubjectProfile: &grpcpb.SubjectProfile{
			SubjectId: "user-123",
			ModulePayloads: []*grpcpb.SubjectModulePayload{
				{
					ModuleKey:     "astrology",
					TheoryVersion: &theoryVersion,
					Facts: []*grpcpb.SubjectFact{
						{FactKey: "sun_sign", Values: []string{"Scorpio"}},
						{FactKey: "moon_sign", Values: []string{"雙魚"}},
					},
				},
			},
		},
		Text: "請分析這個人",
	})
	if err != nil {
		t.Fatalf("ProfileConsult returned error: %v", err)
	}
	if !response.GetStatus() {
		t.Fatalf("expected successful profile consult, got %+v", response)
	}
	if response.GetResponse() == "" {
		t.Fatalf("expected non-empty response, got %+v", response)
	}
}
