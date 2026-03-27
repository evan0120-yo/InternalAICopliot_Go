package infra

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	appsCollection             = "apps"
	appPromptConfigsCollection = "appPromptConfigs"
	buildersCollection         = "builders"
	builderSourcesCollection   = "sources"
	sourceRagsCollection       = "sourceRags"
	templatesCollection        = "templates"
	templateRagsCollection     = "templateRags"
	storeMetadataCollection    = "_meta"
	storeCountersDocument      = "counters"
	sourceLookupCollection     = "_sourceLookup"
)

type StoreSeedData struct {
	Apps             []AppAccess       `json:"apps"`
	AppPromptConfigs []AppPromptConfig `json:"appPromptConfigs"`
	Builders         []BuilderConfig   `json:"builders"`
	Sources          []Source          `json:"sources"`
	Rags             []RagSupplement   `json:"rags"`
	Templates        []Template        `json:"templates"`
	TemplateRags     []TemplateRag     `json:"templateRags"`
	NextSourceID     int64             `json:"nextSourceId"`
	NextRagID        int64             `json:"nextRagId"`
	NextTemplateID   int64             `json:"nextTemplateId"`
	NextTemplateRag  int64             `json:"nextTemplateRagId"`
}

type storeCounters struct {
	NextSourceID    int64 `firestore:"nextSourceId"`
	NextRagID       int64 `firestore:"nextRagId"`
	NextTemplateID  int64 `firestore:"nextTemplateId"`
	NextTemplateRag int64 `firestore:"nextTemplateRagId"`
}

type sourceLookup struct {
	SourceID  int64 `firestore:"sourceId"`
	BuilderID int   `firestore:"builderId"`
}

// StoreOptions controls Firestore bootstrap behavior.
type StoreOptions struct {
	ProjectID     string
	EmulatorHost  string
	SeedWhenEmpty bool
	ResetOnStart  bool
	SeedData      StoreSeedData
}

// Store is the Firestore persistence boundary used by all modules.
type Store struct {
	client    *firestore.Client
	projectID string
}

// NewStore keeps the legacy constructor shape used by tests while switching runtime to Firestore.
func NewStore(_ string) (*Store, error) {
	projectID := firestoreProjectIDFromEnv()
	if firestoreEmulatorHostFromEnv() != "" {
		projectID = fmt.Sprintf("%s-test-%d", projectID, time.Now().UnixNano())
	}
	return NewStoreWithOptions(StoreOptions{
		ProjectID:     projectID,
		EmulatorHost:  firestoreEmulatorHostFromEnv(),
		SeedWhenEmpty: true,
		ResetOnStart:  true,
		SeedData:      DefaultSeedData(),
	})
}

// NewStoreWithOptions creates a Firestore-backed store and applies local bootstrap rules.
func NewStoreWithOptions(options StoreOptions) (*Store, error) {
	projectID := options.ProjectID
	if projectID == "" {
		projectID = firestoreProjectIDFromEnv()
	}
	if projectID == "" {
		return nil, fmt.Errorf("firestore project id is required")
	}

	emulatorHost := options.EmulatorHost
	if emulatorHost == "" {
		emulatorHost = firestoreEmulatorHostFromEnv()
	}

	ctx := context.Background()
	client, err := newFirestoreClient(ctx, projectID, emulatorHost)
	if err != nil {
		return nil, err
	}

	store := &Store{
		client:    client,
		projectID: projectID,
	}

	seedData := options.SeedData
	if len(seedData.Apps) == 0 && len(seedData.Builders) == 0 && len(seedData.Templates) == 0 && len(seedData.Sources) == 0 {
		seedData = DefaultSeedData()
	}

	if options.ResetOnStart {
		if err := store.resetAndSeed(ctx, seedData); err != nil {
			_ = client.Close()
			return nil, err
		}
		if err := store.ensureMetadata(ctx); err != nil {
			_ = client.Close()
			return nil, err
		}
		return store, nil
	}

	if options.SeedWhenEmpty {
		isEmpty, err := store.isEmpty(ctx)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		if isEmpty {
			if err := store.seed(ctx, seedData); err != nil {
				_ = client.Close()
				return nil, err
			}
		}
	}
	if err := store.ensureApps(ctx, seedData.Apps); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := store.ensureMetadata(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	return store, nil
}

// Close releases the underlying Firestore client.
func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func firestoreProjectIDFromEnv() string {
	if value := os.Getenv("INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID"); value != "" {
		return value
	}
	if value := os.Getenv("REWARDBRIDGE_FIRESTORE_PROJECT_ID"); value != "" {
		return value
	}
	if value := os.Getenv("GCLOUD_PROJECT"); value != "" {
		return value
	}
	if value := os.Getenv("GOOGLE_CLOUD_PROJECT"); value != "" {
		return value
	}
	return "dailo-467502"
}

func firestoreEmulatorHostFromEnv() string {
	if value := os.Getenv("INTERNAL_AI_COPILOT_FIRESTORE_EMULATOR_HOST"); value != "" {
		return value
	}
	if value := os.Getenv("REWARDBRIDGE_FIRESTORE_EMULATOR_HOST"); value != "" {
		return value
	}
	if value := os.Getenv("FIRESTORE_EMULATOR_HOST"); value != "" {
		return value
	}
	return "localhost:8090"
}

func newFirestoreClient(ctx context.Context, projectID, emulatorHost string) (*firestore.Client, error) {
	options := make([]option.ClientOption, 0, 3)
	if emulatorHost != "" {
		options = append(options,
			option.WithEndpoint(emulatorHost),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	}
	return firestore.NewClient(ctx, projectID, options...)
}

func (s *Store) BuildersContext(ctx context.Context) ([]BuilderConfig, error) {
	docs, err := s.client.Collection(buildersCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	builders := make([]BuilderConfig, 0, len(docs))
	for _, doc := range docs {
		var builder BuilderConfig
		if err := doc.DataTo(&builder); err != nil {
			return nil, err
		}
		builders = append(builders, builder)
	}
	SortByOrderThenID(builders, func(BuilderConfig) int { return 0 }, func(builder BuilderConfig) int64 { return int64(builder.BuilderID) })
	return builders, nil
}

func (s *Store) ActiveBuildersContext(ctx context.Context) ([]BuilderConfig, error) {
	builders, err := s.BuildersContext(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]BuilderConfig, 0, len(builders))
	for _, builder := range builders {
		if builder.Active {
			filtered = append(filtered, builder)
		}
	}
	return filtered, nil
}

// BuilderByIDContext reads a builder while respecting cancellation.
func (s *Store) BuilderByIDContext(ctx context.Context, builderID int) (BuilderConfig, bool, error) {
	snapshot, err := s.client.Collection(buildersCollection).Doc(strconv.Itoa(builderID)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return BuilderConfig{}, false, nil
		}
		return BuilderConfig{}, false, err
	}

	var builder BuilderConfig
	if err := snapshot.DataTo(&builder); err != nil {
		return BuilderConfig{}, false, err
	}
	return builder, true, nil
}

func (s *Store) BuilderByCodeContext(ctx context.Context, builderCode string) (BuilderConfig, bool, error) {
	docs, err := s.client.Collection(buildersCollection).Where("builderCode", "==", builderCode).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return BuilderConfig{}, false, err
	}
	if len(docs) == 0 {
		return BuilderConfig{}, false, nil
	}

	var builder BuilderConfig
	if err := docs[0].DataTo(&builder); err != nil {
		return BuilderConfig{}, false, err
	}
	return builder, true, nil
}

func (s *Store) SaveBuilder(ctx context.Context, builder BuilderConfig) (BuilderConfig, error) {
	_, err := s.client.Collection(buildersCollection).Doc(strconv.Itoa(builder.BuilderID)).Set(ctx, builder)
	if err != nil {
		return BuilderConfig{}, err
	}
	return builder, nil
}

// AppByIDContext reads an external app access record by appId.
func (s *Store) AppByIDContext(ctx context.Context, appID string) (AppAccess, bool, error) {
	snapshot, err := s.client.Collection(appsCollection).Doc(strings.TrimSpace(appID)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return AppAccess{}, false, nil
		}
		return AppAccess{}, false, err
	}

	var app AppAccess
	if err := snapshot.DataTo(&app); err != nil {
		return AppAccess{}, false, err
	}
	return app, true, nil
}

// AppPromptConfigByAppIDContext reads one app prompt config by appId.
func (s *Store) AppPromptConfigByAppIDContext(ctx context.Context, appID string) (AppPromptConfig, bool, error) {
	snapshot, err := s.client.Collection(appPromptConfigsCollection).Doc(strings.TrimSpace(appID)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return AppPromptConfig{}, false, nil
		}
		return AppPromptConfig{}, false, err
	}

	var config AppPromptConfig
	if err := snapshot.DataTo(&config); err != nil {
		return AppPromptConfig{}, false, err
	}
	return config, true, nil
}

// SourcesByBuilderIDContext reads builder sources while respecting cancellation.
func (s *Store) SourcesByBuilderIDContext(ctx context.Context, builderID int) ([]Source, error) {
	docs, err := s.builderDoc(builderID).Collection(builderSourcesCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	sources := make([]Source, 0, len(docs))
	for _, doc := range docs {
		var source Source
		if err := doc.DataTo(&source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}

	SortByOrderThenID(sources, func(source Source) int { return source.OrderNo }, func(source Source) int64 { return source.SourceID })
	return sources, nil
}

// RagsBySourceIDContext reads source supplements while respecting cancellation.
func (s *Store) RagsBySourceIDContext(ctx context.Context, sourceID int64) ([]RagSupplement, error) {
	sourceDoc, err := s.sourceDocBySourceID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if sourceDoc == nil {
		return nil, nil
	}

	docs, err := sourceDoc.Collection(sourceRagsCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	rags := make([]RagSupplement, 0, len(docs))
	for _, doc := range docs {
		var rag RagSupplement
		if err := doc.DataTo(&rag); err != nil {
			return nil, err
		}
		rags = append(rags, rag)
	}

	SortByOrderThenID(rags, func(rag RagSupplement) int { return rag.OrderNo }, func(rag RagSupplement) int64 { return rag.RagID })
	return rags, nil
}

func (s *Store) ReplaceBuilderGraph(ctx context.Context, builderID int, sources []Source, rags []RagSupplement) error {
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		countersSnapshot, err := tx.Get(s.countersDoc())
		if err != nil {
			return err
		}
		var counters storeCounters
		if err := countersSnapshot.DataTo(&counters); err != nil {
			return err
		}

		sourceIter := tx.Documents(s.builderDoc(builderID).Collection(builderSourcesCollection))
		sourceDocs := make([]*firestore.DocumentSnapshot, 0)
		for {
			doc, err := sourceIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			sourceDocs = append(sourceDocs, doc)
		}

		ragDocsBySourcePath := make(map[string][]*firestore.DocumentSnapshot, len(sourceDocs))
		for _, doc := range sourceDocs {
			var existingSource Source
			if err := doc.DataTo(&existingSource); err != nil {
				return err
			}
			if existingSource.SystemBlock {
				continue
			}

			ragIter := tx.Documents(doc.Ref.Collection(sourceRagsCollection))
			ragDocs := make([]*firestore.DocumentSnapshot, 0)
			for {
				ragDoc, err := ragIter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
				ragDocs = append(ragDocs, ragDoc)
			}
			ragDocsBySourcePath[doc.Ref.Path] = ragDocs
		}

		for _, doc := range sourceDocs {
			var existingSource Source
			if err := doc.DataTo(&existingSource); err != nil {
				return err
			}
			if existingSource.SystemBlock {
				continue
			}

			for _, ragDoc := range ragDocsBySourcePath[doc.Ref.Path] {
				if err := tx.Delete(ragDoc.Ref); err != nil {
					return err
				}
			}
			if err := tx.Delete(s.sourceLookupDoc(existingSource.SourceID)); err != nil {
				return err
			}
			if err := tx.Delete(doc.Ref); err != nil {
				return err
			}
		}

		resolvedSourceIDs := make(map[int64]int64, len(sources))
		for _, source := range sources {
			counters.NextSourceID++
			resolvedSourceIDs[source.SourceID] = counters.NextSourceID
		}

		for index := range sources {
			source := sources[index]
			sourceID := resolvedSourceIDs[source.SourceID]
			source.SourceID = sourceID
			source.BuilderID = builderID
			if len(source.SourceIDs) > 0 {
				rewritten := make([]int64, 0, len(source.SourceIDs))
				for _, referencedID := range source.SourceIDs {
					resolvedID, ok := resolvedSourceIDs[referencedID]
					if !ok {
						return fmt.Errorf("unknown source reference %d", referencedID)
					}
					rewritten = append(rewritten, resolvedID)
				}
				source.SourceIDs = rewritten
			}

			sourceDoc := s.builderDoc(builderID).Collection(builderSourcesCollection).Doc(strconv.FormatInt(sourceID, 10))
			if err := tx.Set(sourceDoc, source); err != nil {
				return err
			}
			if err := tx.Set(s.sourceLookupDoc(sourceID), sourceLookup{
				SourceID:  sourceID,
				BuilderID: builderID,
			}); err != nil {
				return err
			}

			placeholderSourceID := int64(-(index + 1))
			for ragIndex := range rags {
				if rags[ragIndex].SourceID != placeholderSourceID {
					continue
				}
				counters.NextRagID++
				rag := rags[ragIndex]
				rag.RagID = counters.NextRagID
				rag.SourceID = sourceID
				if err := tx.Set(sourceDoc.Collection(sourceRagsCollection).Doc(strconv.FormatInt(rag.RagID, 10)), rag); err != nil {
					return err
				}
			}
		}

		return tx.Set(s.countersDoc(), counters)
	})
}

func (s *Store) TemplatesContext(ctx context.Context) ([]Template, error) {
	docs, err := s.client.Collection(templatesCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	templates := make([]Template, 0, len(docs))
	for _, doc := range docs {
		var template Template
		if err := doc.DataTo(&template); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}

	SortByOrderThenID(templates, func(template Template) int { return template.OrderNo }, func(template Template) int64 { return template.TemplateID })
	return templates, nil
}

func (s *Store) TemplateByIDContext(ctx context.Context, templateID int64) (Template, bool, error) {
	snapshot, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(templateID, 10)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return Template{}, false, nil
		}
		return Template{}, false, err
	}

	var template Template
	if err := snapshot.DataTo(&template); err != nil {
		return Template{}, false, err
	}
	return template, true, nil
}

func (s *Store) TemplateByKeyContext(ctx context.Context, templateKey string) (Template, bool, error) {
	docs, err := s.client.Collection(templatesCollection).Where("templateKey", "==", templateKey).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return Template{}, false, err
	}
	if len(docs) == 0 {
		return Template{}, false, nil
	}

	var template Template
	if err := docs[0].DataTo(&template); err != nil {
		return Template{}, false, err
	}
	return template, true, nil
}

func (s *Store) TemplateRagsByTemplateIDContext(ctx context.Context, templateID int64) ([]TemplateRag, error) {
	docs, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(templateID, 10)).Collection(templateRagsCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	rags := make([]TemplateRag, 0, len(docs))
	for _, doc := range docs {
		var rag TemplateRag
		if err := doc.DataTo(&rag); err != nil {
			return nil, err
		}
		rags = append(rags, rag)
	}

	SortByOrderThenID(rags, func(rag TemplateRag) int { return rag.OrderNo }, func(rag TemplateRag) int64 { return rag.TemplateRagID })
	return rags, nil
}

func (s *Store) SaveTemplate(ctx context.Context, template Template, rags []TemplateRag) (Template, []TemplateRag, error) {
	savedTemplate := template
	savedRags := make([]TemplateRag, 0, len(rags))

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		countersSnapshot, err := tx.Get(s.countersDoc())
		if err != nil {
			return err
		}
		var counters storeCounters
		if err := countersSnapshot.DataTo(&counters); err != nil {
			return err
		}

		localTemplate := template
		if localTemplate.TemplateID == 0 {
			counters.NextTemplateID++
			localTemplate.TemplateID = counters.NextTemplateID
		}
		templateDoc := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(localTemplate.TemplateID, 10))
		ragIter := tx.Documents(templateDoc.Collection(templateRagsCollection))
		existingRagDocs := make([]*firestore.DocumentSnapshot, 0)
		for {
			ragDoc, err := ragIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			existingRagDocs = append(existingRagDocs, ragDoc)
		}

		if err := tx.Set(templateDoc, localTemplate); err != nil {
			return err
		}
		for _, ragDoc := range existingRagDocs {
			if err := tx.Delete(ragDoc.Ref); err != nil {
				return err
			}
		}

		localSavedRags := make([]TemplateRag, 0, len(rags))
		for _, requestedRag := range rags {
			counters.NextTemplateRag++
			rag := requestedRag
			rag.TemplateRagID = counters.NextTemplateRag
			rag.TemplateID = localTemplate.TemplateID
			if err := tx.Set(templateDoc.Collection(templateRagsCollection).Doc(strconv.FormatInt(rag.TemplateRagID, 10)), rag); err != nil {
				return err
			}
			localSavedRags = append(localSavedRags, rag)
		}

		if err := tx.Set(s.countersDoc(), counters); err != nil {
			return err
		}

		savedTemplate = localTemplate
		savedRags = localSavedRags
		return nil
	})
	if err != nil {
		return Template{}, nil, err
	}

	return savedTemplate, savedRags, nil
}

func (s *Store) ReorderTemplates(ctx context.Context, templateIDs []int64) error {
	for index, templateID := range templateIDs {
		if _, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(templateID, 10)).Update(ctx, []firestore.Update{
			{Path: "orderNo", Value: index + 1},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteTemplate(ctx context.Context, templateID int64) error {
	templateDoc := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(templateID, 10))

	ragDocs, err := templateDoc.Collection(templateRagsCollection).Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	for _, doc := range ragDocs {
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return err
		}
	}

	if _, err := templateDoc.Delete(ctx); err != nil {
		return err
	}

	builders, err := s.BuildersContext(ctx)
	if err != nil {
		return err
	}
	for _, builder := range builders {
		sourceDocs, err := s.builderDoc(builder.BuilderID).Collection(builderSourcesCollection).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, doc := range sourceDocs {
			var source Source
			if err := doc.DataTo(&source); err != nil {
				return err
			}
			if source.CopiedFromTemplateID == nil || *source.CopiedFromTemplateID != templateID {
				continue
			}
			if _, err := doc.Ref.Set(ctx, map[string]any{
				"copiedFromTemplateId":          firestore.Delete,
				"copiedFromTemplateKey":         firestore.Delete,
				"copiedFromTemplateName":        firestore.Delete,
				"copiedFromTemplateDescription": firestore.Delete,
				"copiedFromTemplateGroupKey":    firestore.Delete,
			}, firestore.MergeAll); err != nil {
				return err
			}
		}
	}

	templates, err := s.TemplatesContext(ctx)
	if err != nil {
		return err
	}
	SortByOrderThenID(templates, func(template Template) int { return template.OrderNo }, func(template Template) int64 { return template.TemplateID })
	for index := range templates {
		if _, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(templates[index].TemplateID, 10)).Update(ctx, []firestore.Update{
			{Path: "orderNo", Value: index + 1},
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) isEmpty(ctx context.Context) (bool, error) {
	builders, err := s.client.Collection(buildersCollection).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return false, err
	}
	if len(builders) > 0 {
		return false, nil
	}

	templates, err := s.client.Collection(templatesCollection).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return false, err
	}
	return len(templates) == 0, nil
}

func (s *Store) resetAndSeed(ctx context.Context, data StoreSeedData) error {
	if err := s.clearBuildersCollection(ctx); err != nil {
		return err
	}
	if err := s.clearFlatRootCollection(ctx, s.client.Collection(appsCollection)); err != nil {
		return err
	}
	if err := s.clearFlatRootCollection(ctx, s.client.Collection(appPromptConfigsCollection)); err != nil {
		return err
	}
	if err := s.clearTemplatesCollection(ctx); err != nil {
		return err
	}
	if err := s.clearFlatRootCollection(ctx, s.client.Collection(sourceLookupCollection)); err != nil {
		return err
	}
	if err := s.clearFlatRootCollection(ctx, s.client.Collection(storeMetadataCollection)); err != nil {
		return err
	}
	return s.seed(ctx, data)
}

func (s *Store) seed(ctx context.Context, data StoreSeedData) error {
	for _, app := range data.Apps {
		if _, err := s.client.Collection(appsCollection).Doc(app.AppID).Set(ctx, app); err != nil {
			return err
		}
	}

	for _, config := range data.AppPromptConfigs {
		if _, err := s.client.Collection(appPromptConfigsCollection).Doc(config.AppID).Set(ctx, config); err != nil {
			return err
		}
	}

	for _, builder := range data.Builders {
		if _, err := s.client.Collection(buildersCollection).Doc(strconv.Itoa(builder.BuilderID)).Set(ctx, builder); err != nil {
			return err
		}
	}

	for _, source := range data.Sources {
		sourceDoc := s.builderDoc(source.BuilderID).Collection(builderSourcesCollection).Doc(strconv.FormatInt(source.SourceID, 10))
		if _, err := sourceDoc.Set(ctx, source); err != nil {
			return err
		}
		if _, err := s.sourceLookupDoc(source.SourceID).Set(ctx, sourceLookup{
			SourceID:  source.SourceID,
			BuilderID: source.BuilderID,
		}); err != nil {
			return err
		}
	}

	for _, rag := range data.Rags {
		sourceDoc, err := s.sourceDocBySourceID(ctx, rag.SourceID)
		if err != nil {
			return err
		}
		if sourceDoc == nil {
			return fmt.Errorf("seed source %d not found for rag %d", rag.SourceID, rag.RagID)
		}
		if _, err := sourceDoc.Collection(sourceRagsCollection).Doc(strconv.FormatInt(rag.RagID, 10)).Set(ctx, rag); err != nil {
			return err
		}
	}

	for _, template := range data.Templates {
		if _, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(template.TemplateID, 10)).Set(ctx, template); err != nil {
			return err
		}
	}

	for _, rag := range data.TemplateRags {
		templateDoc := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(rag.TemplateID, 10))
		if _, err := templateDoc.Collection(templateRagsCollection).Doc(strconv.FormatInt(rag.TemplateRagID, 10)).Set(ctx, rag); err != nil {
			return err
		}
	}

	_, err := s.countersDoc().Set(ctx, storeCounters{
		NextSourceID:    data.NextSourceID,
		NextRagID:       data.NextRagID,
		NextTemplateID:  data.NextTemplateID,
		NextTemplateRag: data.NextTemplateRag,
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) clearFlatRootCollection(ctx context.Context, collection *firestore.CollectionRef) error {
	docs, err := collection.Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) clearBuildersCollection(ctx context.Context) error {
	builderDocs, err := s.client.Collection(buildersCollection).Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	for _, builderDoc := range builderDocs {
		sourceDocs, err := builderDoc.Ref.Collection(builderSourcesCollection).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, sourceDoc := range sourceDocs {
			ragDocs, err := sourceDoc.Ref.Collection(sourceRagsCollection).Documents(ctx).GetAll()
			if err != nil {
				return err
			}
			for _, ragDoc := range ragDocs {
				if _, err := ragDoc.Ref.Delete(ctx); err != nil {
					return err
				}
			}
			if _, err := sourceDoc.Ref.Delete(ctx); err != nil {
				return err
			}
		}
		if _, err := builderDoc.Ref.Delete(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) clearTemplatesCollection(ctx context.Context) error {
	templateDocs, err := s.client.Collection(templatesCollection).Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	for _, templateDoc := range templateDocs {
		ragDocs, err := templateDoc.Ref.Collection(templateRagsCollection).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, ragDoc := range ragDocs {
			if _, err := ragDoc.Ref.Delete(ctx); err != nil {
				return err
			}
		}
		if _, err := templateDoc.Ref.Delete(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) sourceDocBySourceID(ctx context.Context, sourceID int64) (*firestore.DocumentRef, error) {
	lookupSnapshot, err := s.sourceLookupDoc(sourceID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var lookup sourceLookup
	if err := lookupSnapshot.DataTo(&lookup); err != nil {
		return nil, err
	}
	return s.builderDoc(lookup.BuilderID).Collection(builderSourcesCollection).Doc(strconv.FormatInt(sourceID, 10)), nil
}

func (s *Store) maxSourceID(ctx context.Context) (int64, error) {
	counters, err := s.readCounters(ctx)
	if err != nil {
		return 0, err
	}
	return counters.NextSourceID, nil
}

func (s *Store) maxRagID(ctx context.Context) (int64, error) {
	counters, err := s.readCounters(ctx)
	if err != nil {
		return 0, err
	}
	return counters.NextRagID, nil
}

func (s *Store) maxTemplateID(ctx context.Context) (int64, error) {
	counters, err := s.readCounters(ctx)
	if err != nil {
		return 0, err
	}
	return counters.NextTemplateID, nil
}

func (s *Store) maxTemplateRagID(ctx context.Context) (int64, error) {
	counters, err := s.readCounters(ctx)
	if err != nil {
		return 0, err
	}
	return counters.NextTemplateRag, nil
}

func (s *Store) builderDoc(builderID int) *firestore.DocumentRef {
	return s.client.Collection(buildersCollection).Doc(strconv.Itoa(builderID))
}

func (s *Store) countersDoc() *firestore.DocumentRef {
	return s.client.Collection(storeMetadataCollection).Doc(storeCountersDocument)
}

func (s *Store) sourceLookupDoc(sourceID int64) *firestore.DocumentRef {
	return s.client.Collection(sourceLookupCollection).Doc(strconv.FormatInt(sourceID, 10))
}

func (s *Store) readCounters(ctx context.Context) (storeCounters, error) {
	snapshot, err := s.countersDoc().Get(ctx)
	if err != nil {
		return storeCounters{}, err
	}
	var counters storeCounters
	if err := snapshot.DataTo(&counters); err != nil {
		return storeCounters{}, err
	}
	return counters, nil
}

func (s *Store) ensureMetadata(ctx context.Context) error {
	counters := storeCounters{}
	builders, err := s.BuildersContext(ctx)
	if err != nil {
		return err
	}

	lookups := make([]sourceLookup, 0)
	for _, builder := range builders {
		sourceDocs, err := s.builderDoc(builder.BuilderID).Collection(builderSourcesCollection).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, sourceDoc := range sourceDocs {
			var source Source
			if err := sourceDoc.DataTo(&source); err != nil {
				return err
			}
			if source.SourceID > counters.NextSourceID {
				counters.NextSourceID = source.SourceID
			}
			lookups = append(lookups, sourceLookup{
				SourceID:  source.SourceID,
				BuilderID: builder.BuilderID,
			})

			ragDocs, err := sourceDoc.Ref.Collection(sourceRagsCollection).Documents(ctx).GetAll()
			if err != nil {
				return err
			}
			for _, ragDoc := range ragDocs {
				var rag RagSupplement
				if err := ragDoc.DataTo(&rag); err != nil {
					return err
				}
				if rag.RagID > counters.NextRagID {
					counters.NextRagID = rag.RagID
				}
			}
		}
	}

	templates, err := s.TemplatesContext(ctx)
	if err != nil {
		return err
	}
	for _, template := range templates {
		if template.TemplateID > counters.NextTemplateID {
			counters.NextTemplateID = template.TemplateID
		}
		ragDocs, err := s.client.Collection(templatesCollection).Doc(strconv.FormatInt(template.TemplateID, 10)).Collection(templateRagsCollection).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, ragDoc := range ragDocs {
			var rag TemplateRag
			if err := ragDoc.DataTo(&rag); err != nil {
				return err
			}
			if rag.TemplateRagID > counters.NextTemplateRag {
				counters.NextTemplateRag = rag.TemplateRagID
			}
		}
	}

	if err := s.clearFlatRootCollection(ctx, s.client.Collection(sourceLookupCollection)); err != nil {
		return err
	}
	for _, lookup := range lookups {
		if _, err := s.sourceLookupDoc(lookup.SourceID).Set(ctx, lookup); err != nil {
			return err
		}
	}
	_, err = s.countersDoc().Set(ctx, counters)
	return err
}

func (s *Store) ensureApps(ctx context.Context, apps []AppAccess) error {
	for _, app := range apps {
		if strings.TrimSpace(app.AppID) == "" {
			continue
		}
		_, exists, err := s.AppByIDContext(ctx, app.AppID)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.client.Collection(appsCollection).Doc(app.AppID).Set(ctx, app); err != nil {
			return err
		}
	}
	return nil
}
