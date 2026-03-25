package gatekeeper

import (
	"context"
	"net"
	"net/http"
	"strings"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
)

var supportedFileExtensions = map[string]struct{}{
	"pdf":  {},
	"doc":  {},
	"docx": {},
	"jpg":  {},
	"jpeg": {},
	"png":  {},
	"webp": {},
	"gif":  {},
	"bmp":  {},
}

const ExternalAppIDHeader = "X-App-Id"

// GuardService validates gatekeeper requests.
type GuardService struct {
	config infra.Config
	store  *infra.Store
}

// NewGuardService builds the gatekeeper validation service.
func NewGuardService(config infra.Config, store *infra.Store) *GuardService {
	return &GuardService{
		config: config,
		store:  store,
	}
}

// MultipartMemoryLimit returns the maximum multipart memory budget used by the handler.
func (s *GuardService) MultipartMemoryLimit() int64 {
	if s.config.ConsultMaxTotalSize > 0 {
		return s.config.ConsultMaxTotalSize
	}
	return 64 << 20
}

// ResolveClientIP resolves the request client IP.
func (s *GuardService) ResolveClientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if value != "" {
			return value
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// ValidateConsult validates public consult input.
func (s *GuardService) ValidateConsult(ctx context.Context, builderID int, outputFormat string, attachments []infra.Attachment, clientIP string) (infra.BuilderConfig, *infra.OutputFormat, error) {
	if strings.TrimSpace(clientIP) == "" {
		return infra.BuilderConfig{}, nil, infra.NewError("CLIENT_IP_MISSING", "Client IP could not be resolved.", http.StatusBadRequest)
	}
	builderConfig, err := s.validateActiveBuilder(ctx, builderID)
	if err != nil {
		return infra.BuilderConfig{}, nil, err
	}

	var parsedFormat *infra.OutputFormat
	if strings.TrimSpace(outputFormat) != "" {
		format, ok := infra.ParseOutputFormat(outputFormat)
		if !ok {
			return infra.BuilderConfig{}, nil, infra.NewError("UNSUPPORTED_OUTPUT_FORMAT", "Only markdown and xlsx output formats are supported.", http.StatusBadRequest)
		}
		parsedFormat = &format
	}

	actualFiles := 0
	totalBytes := int64(0)
	for _, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		actualFiles++
		if actualFiles > s.config.ConsultMaxFiles {
			return infra.BuilderConfig{}, nil, infra.NewError("FILE_COUNT_EXCEEDED", "Uploaded file count exceeds the configured limit.", http.StatusBadRequest)
		}
		if int64(len(attachment.Data)) > s.config.ConsultMaxFileSize {
			return infra.BuilderConfig{}, nil, infra.NewError("FILE_SIZE_EXCEEDED", "Uploaded file exceeds the configured per-file size limit.", http.StatusBadRequest)
		}
		totalBytes += int64(len(attachment.Data))
		if totalBytes > s.config.ConsultMaxTotalSize {
			return infra.BuilderConfig{}, nil, infra.NewError("FILE_TOTAL_SIZE_EXCEEDED", "Uploaded files exceed the configured total size limit.", http.StatusBadRequest)
		}
		extension := strings.ToLower(strings.TrimPrefix(fileExtension(attachment.FileName), "."))
		if _, ok := supportedFileExtensions[extension]; !ok {
			return infra.BuilderConfig{}, nil, infra.NewError("UNSUPPORTED_FILE_TYPE", "Only PDF, DOC, DOCX, JPG, JPEG, PNG, WEBP, GIF, and BMP files are supported.", http.StatusBadRequest)
		}
	}

	return builderConfig, parsedFormat, nil
}

// ValidateProfileConsult validates structured profile consult input.
func (s *GuardService) ValidateProfileConsult(ctx context.Context, builderID int, analysisModules []string, subjectProfile *builder.SubjectProfile, text, clientIP string) (infra.BuilderConfig, []string, *builder.SubjectProfile, error) {
	if strings.TrimSpace(clientIP) == "" {
		return infra.BuilderConfig{}, nil, nil, infra.NewError("CLIENT_IP_MISSING", "Client IP could not be resolved.", http.StatusBadRequest)
	}
	builderConfig, err := s.validateActiveBuilder(ctx, builderID)
	if err != nil {
		return infra.BuilderConfig{}, nil, nil, err
	}

	normalizedModules, err := builder.NormalizeAnalysisModules(analysisModules)
	if err != nil {
		return infra.BuilderConfig{}, nil, nil, err
	}
	normalizedProfile, err := normalizeSubjectProfile(subjectProfile, normalizedModules)
	if err != nil {
		return infra.BuilderConfig{}, nil, nil, err
	}
	if len(normalizedModules) == 0 && strings.TrimSpace(text) == "" && normalizedProfile == nil {
		return infra.BuilderConfig{}, nil, nil, infra.NewError("PROFILE_INPUT_EMPTY", "analysisModules, subjectProfile, or text is required.", http.StatusBadRequest)
	}

	return builderConfig, normalizedModules, normalizedProfile, nil
}

// ValidateExternalApp validates the external caller app identity.
func (s *GuardService) ValidateExternalApp(ctx context.Context, appID string) (infra.AppAccess, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return infra.AppAccess{}, infra.NewError("APP_ID_MISSING", "appId is required.", http.StatusBadRequest)
	}

	app, ok, err := s.store.AppByIDContext(ctx, appID)
	if err != nil {
		return infra.AppAccess{}, err
	}
	if !ok {
		return infra.AppAccess{}, infra.NewError("APP_NOT_FOUND", "Requested app does not exist.", http.StatusBadRequest)
	}
	if !app.Active {
		return infra.AppAccess{}, infra.NewError("APP_INACTIVE", "Requested app is inactive.", http.StatusForbidden)
	}
	return app, nil
}

// ValidateExternalConsult validates an external app consult request.
func (s *GuardService) ValidateExternalConsult(ctx context.Context, appID string, builderID int, outputFormat string, attachments []infra.Attachment, clientIP string) (infra.AppAccess, infra.BuilderConfig, *infra.OutputFormat, error) {
	app, err := s.ValidateExternalApp(ctx, appID)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, err
	}

	builderConfig, parsedFormat, err := s.ValidateConsult(ctx, builderID, outputFormat, attachments, clientIP)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, err
	}
	if !appAllowsBuilder(app, builderID) {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, infra.NewError("APP_BUILDER_FORBIDDEN", "Requested app is not allowed to use this builder.", http.StatusForbidden)
	}

	return app, builderConfig, parsedFormat, nil
}

// ValidateExternalProfileConsult validates an external app profile consult request.
func (s *GuardService) ValidateExternalProfileConsult(ctx context.Context, appID string, builderID int, analysisModules []string, subjectProfile *builder.SubjectProfile, text, clientIP string) (infra.AppAccess, infra.BuilderConfig, []string, *builder.SubjectProfile, error) {
	app, err := s.ValidateExternalApp(ctx, appID)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, nil, err
	}

	builderConfig, normalizedModules, normalizedProfile, err := s.ValidateProfileConsult(ctx, builderID, analysisModules, subjectProfile, text, clientIP)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, nil, err
	}
	if !appAllowsBuilder(app, builderID) {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, nil, infra.NewError("APP_BUILDER_FORBIDDEN", "Requested app is not allowed to use this builder.", http.StatusForbidden)
	}

	return app, builderConfig, normalizedModules, normalizedProfile, nil
}

func (s *GuardService) validateActiveBuilder(ctx context.Context, builderID int) (infra.BuilderConfig, error) {
	if builderID == 0 {
		return infra.BuilderConfig{}, infra.NewError("BUILDER_ID_MISSING", "builderId is required.", http.StatusBadRequest)
	}
	builderConfig, ok, err := s.store.BuilderByIDContext(ctx, builderID)
	if err != nil {
		return infra.BuilderConfig{}, err
	}
	if !ok {
		return infra.BuilderConfig{}, infra.NewError("BUILDER_NOT_FOUND", "Requested builder does not exist.", http.StatusBadRequest)
	}
	if !builderConfig.Active {
		return infra.BuilderConfig{}, infra.NewError("BUILDER_INACTIVE", "Requested builder is inactive.", http.StatusForbidden)
	}
	return builderConfig, nil
}

func normalizeSubjectProfile(subjectProfile *builder.SubjectProfile, analysisModules []string) (*builder.SubjectProfile, error) {
	if subjectProfile == nil {
		return nil, nil
	}

	allowedModules := make(map[string]struct{}, len(analysisModules))
	for _, moduleKey := range analysisModules {
		allowedModules[moduleKey] = struct{}{}
	}

	normalized := &builder.SubjectProfile{
		SubjectID: strings.TrimSpace(subjectProfile.SubjectID),
	}
	if normalized.SubjectID == "" && len(subjectProfile.ModulePayloads) == 0 {
		return nil, nil
	}
	if normalized.SubjectID == "" {
		return nil, infra.NewError("SUBJECT_ID_MISSING", "subjectProfile.subjectId is required when subjectProfile is present.", http.StatusBadRequest)
	}

	seenModules := make(map[string]struct{}, len(subjectProfile.ModulePayloads))
	normalized.ModulePayloads = make([]builder.SubjectModulePayload, 0, len(subjectProfile.ModulePayloads))
	for _, payload := range subjectProfile.ModulePayloads {
		moduleKey, err := builder.NormalizeProfileModuleKey(payload.ModuleKey)
		if err != nil {
			return nil, err
		}
		if _, ok := seenModules[moduleKey]; ok {
			return nil, infra.NewError("DUPLICATE_MODULE_PAYLOAD", "subjectProfile.modulePayloads must not repeat moduleKey.", http.StatusBadRequest)
		}
		if _, ok := allowedModules[moduleKey]; !ok {
			return nil, infra.NewError("PROFILE_MODULE_NOT_ALLOWED", "subjectProfile.modulePayloads moduleKey must exist in analysisModules.", http.StatusBadRequest)
		}
		seenModules[moduleKey] = struct{}{}

		normalizedPayload := builder.SubjectModulePayload{
			ModuleKey:     moduleKey,
			TheoryVersion: normalizeTheoryVersion(payload.TheoryVersion),
			Facts:         make([]builder.SubjectFact, 0, len(payload.Facts)),
		}
		if payload.TheoryVersion != nil && normalizedPayload.TheoryVersion == nil {
			return nil, infra.NewError("THEORY_VERSION_MISSING", "subjectProfile.modulePayloads.theoryVersion must not be blank when provided.", http.StatusBadRequest)
		}
		seenFacts := make(map[string]struct{}, len(payload.Facts))
		for _, fact := range payload.Facts {
			factKey := strings.TrimSpace(fact.FactKey)
			if factKey == "" {
				return nil, infra.NewError("SUBJECT_FACT_KEY_MISSING", "subjectProfile facts require factKey.", http.StatusBadRequest)
			}
			if _, ok := seenFacts[factKey]; ok {
				return nil, infra.NewError("DUPLICATE_FACT_KEY", "subjectProfile facts must not repeat factKey within one module.", http.StatusBadRequest)
			}
			seenFacts[factKey] = struct{}{}

			values := make([]string, 0, len(fact.Values))
			for _, value := range fact.Values {
				trimmed := strings.TrimSpace(value)
				if trimmed == "" {
					return nil, infra.NewError("SUBJECT_FACT_VALUE_MISSING", "subjectProfile fact values must not be blank.", http.StatusBadRequest)
				}
				values = append(values, trimmed)
			}
			if len(values) == 0 {
				return nil, infra.NewError("SUBJECT_FACT_VALUE_MISSING", "subjectProfile facts require at least one value.", http.StatusBadRequest)
			}

			normalizedPayload.Facts = append(normalizedPayload.Facts, builder.SubjectFact{
				FactKey: factKey,
				Values:  values,
			})
		}
		normalized.ModulePayloads = append(normalized.ModulePayloads, normalizedPayload)
	}

	return normalized, nil
}

func normalizeTheoryVersion(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func appAllowsBuilder(app infra.AppAccess, builderID int) bool {
	for _, allowedBuilderID := range app.AllowedBuilderIDs {
		if allowedBuilderID == builderID {
			return true
		}
	}
	return false
}

func fileExtension(fileName string) string {
	index := strings.LastIndex(fileName, ".")
	if index < 0 {
		return ""
	}
	return fileName[index:]
}
