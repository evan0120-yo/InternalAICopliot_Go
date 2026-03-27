package infra

import (
	"context"
	"sync"
	"testing"
)

func TestStoreConcurrentReadsRemainStable(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var waitGroup sync.WaitGroup
	for index := 0; index < 32; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if _, ok, err := store.BuilderByIDContext(context.Background(), 1); err != nil || !ok {
				t.Errorf("BuilderByIDContext failed: ok=%v err=%v", ok, err)
			}
			if _, err := store.SourcesByBuilderIDContext(context.Background(), 2); err != nil {
				t.Errorf("SourcesByBuilderIDContext failed: %v", err)
			}
			if _, err := store.RagsBySourceIDContext(context.Background(), 6); err != nil {
				t.Errorf("RagsBySourceIDContext failed: %v", err)
			}
		}()
	}
	waitGroup.Wait()
}

func TestStoreResetOnStartRebuildsSeedData(t *testing.T) {
	projectID := "internal-ai-copilot-store-test-reset"

	store, err := NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		ResetOnStart:  true,
		SeedData:      DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	builder, ok, err := store.BuilderByIDContext(context.Background(), 1)
	if err != nil {
		t.Fatalf("BuilderByIDContext returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected seeded builder to exist")
	}
	builder.Name = "custom local builder"
	if _, err := store.SaveBuilder(context.Background(), builder); err != nil {
		t.Fatalf("SaveBuilder returned error: %v", err)
	}

	persistedStore, err := NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		SeedData:      DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = persistedStore.Close() })

	persistedBuilder, ok, err := persistedStore.BuilderByIDContext(context.Background(), 1)
	if err != nil {
		t.Fatalf("BuilderByIDContext returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted builder to exist")
	}
	if persistedBuilder.Name != "custom local builder" {
		t.Fatalf("expected persisted builder name, got %q", persistedBuilder.Name)
	}

	resetStore, err := NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		ResetOnStart:  true,
		SeedData:      DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = resetStore.Close() })

	resetBuilder, ok, err := resetStore.BuilderByIDContext(context.Background(), 1)
	if err != nil {
		t.Fatalf("BuilderByIDContext returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected reset builder to exist")
	}
	if resetBuilder.Name != "PM 工時估算與建議" {
		t.Fatalf("expected reset seed builder name, got %q", resetBuilder.Name)
	}
}

func TestStoreResetOnStartClearsNestedCollectionsWithoutMetadataListing(t *testing.T) {
	projectID := "internal-ai-copilot-store-test-nested-reset"
	groupKey := "qa"

	store, err := NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		ResetOnStart:  true,
		SeedData:      DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	customTemplate := Template{
		TemplateKey: "metadata-reset-check",
		Name:        "metadata-reset-check",
		GroupKey:    &groupKey,
		OrderNo:     999,
		Prompts:     "prompt",
		Active:      true,
	}
	savedTemplate, savedRags, err := store.SaveTemplate(context.Background(), customTemplate, []TemplateRag{
		{OrderNo: 1, RagType: "static", Title: "rag", Content: "content", RetrievalMode: "full_context"},
	})
	if err != nil {
		t.Fatalf("SaveTemplate returned error: %v", err)
	}
	if len(savedRags) != 1 {
		t.Fatalf("expected one saved rag, got %d", len(savedRags))
	}

	resetStore, err := NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		ResetOnStart:  true,
		SeedData:      DefaultSeedData(),
	})
	if err != nil {
		t.Fatalf("reset NewStoreWithOptions returned error: %v", err)
	}
	t.Cleanup(func() { _ = resetStore.Close() })

	if _, ok, err := resetStore.TemplateByIDContext(context.Background(), savedTemplate.TemplateID); err != nil {
		t.Fatalf("TemplateByIDContext returned error: %v", err)
	} else if ok {
		t.Fatalf("expected custom template %d to be removed during reset", savedTemplate.TemplateID)
	}
}

func TestStoreReadsAppPromptConfig(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	config, ok, err := store.AppPromptConfigByAppIDContext(context.Background(), "linkchat")
	if err != nil {
		t.Fatalf("AppPromptConfigByAppIDContext returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected linkchat prompt config to exist")
	}
	if config.StrategyKey != "linkchat" {
		t.Fatalf("expected linkchat strategy, got %+v", config)
	}
}
