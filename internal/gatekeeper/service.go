package gatekeeper

import (
	"context"
	"fmt"
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
func (s *GuardService) ValidateProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, userText, intentText, clientIP string) (infra.BuilderConfig, *builder.SubjectProfile, error) {
	if strings.TrimSpace(clientIP) == "" {
		return infra.BuilderConfig{}, nil, infra.NewError("CLIENT_IP_MISSING", "Client IP could not be resolved.", http.StatusBadRequest)
	}
	builderConfig, err := s.validateActiveBuilder(ctx, builderID)
	if err != nil {
		return infra.BuilderConfig{}, nil, err
	}

	normalizedProfile, err := normalizeSubjectProfile(appID, subjectProfile)
	if err != nil {
		return infra.BuilderConfig{}, nil, err
	}
	if strings.TrimSpace(userText) == "" && strings.TrimSpace(intentText) == "" && normalizedProfile == nil {
		return infra.BuilderConfig{}, nil, infra.NewError("PROFILE_INPUT_EMPTY", "subjectProfile, userText, or intentText is required.", http.StatusBadRequest)
	}

	return builderConfig, normalizedProfile, nil
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
func (s *GuardService) ValidateExternalProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, userText, intentText, clientIP string) (infra.AppAccess, infra.BuilderConfig, *builder.SubjectProfile, error) {
	app, err := s.ValidateExternalApp(ctx, appID)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, err
	}

	builderConfig, normalizedProfile, err := s.ValidateProfileConsult(ctx, appID, builderID, subjectProfile, userText, intentText, clientIP)
	if err != nil {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, err
	}
	if !appAllowsBuilder(app, builderID) {
		return infra.AppAccess{}, infra.BuilderConfig{}, nil, infra.NewError("APP_BUILDER_FORBIDDEN", "Requested app is not allowed to use this builder.", http.StatusForbidden)
	}

	return app, builderConfig, normalizedProfile, nil
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

func normalizeSubjectProfile(appID string, subjectProfile *builder.SubjectProfile) (*builder.SubjectProfile, error) {
	if subjectProfile == nil {
		return nil, nil
	}

	normalized := &builder.SubjectProfile{
		SubjectID: strings.TrimSpace(subjectProfile.SubjectID),
	}
	if normalized.SubjectID == "" && len(subjectProfile.AnalysisPayloads) == 0 {
		return nil, nil
	}
	if normalized.SubjectID == "" {
		return nil, infra.NewError("SUBJECT_ID_MISSING", "subjectProfile.subjectId is required when subjectProfile is present.", http.StatusBadRequest)
	}

	seenAnalysisTypes := make(map[string]struct{}, len(subjectProfile.AnalysisPayloads))
	normalized.AnalysisPayloads = make([]builder.SubjectAnalysisPayload, 0, len(subjectProfile.AnalysisPayloads))
	for _, payload := range subjectProfile.AnalysisPayloads {
		analysisType, err := builder.NormalizeAnalysisTypeKey(payload.AnalysisType)
		if err != nil {
			return nil, err
		}
		if _, ok := seenAnalysisTypes[analysisType]; ok {
			return nil, infra.NewError("DUPLICATE_ANALYSIS_PAYLOAD", "subjectProfile.analysisPayloads must not repeat analysisType.", http.StatusBadRequest)
		}
		seenAnalysisTypes[analysisType] = struct{}{}

		normalizedPayload := builder.SubjectAnalysisPayload{
			AnalysisType:  analysisType,
			TheoryVersion: normalizeTheoryVersion(payload.TheoryVersion),
			Payload:       cloneAnalysisPayloadValueMap(payload.Payload),
		}
		if payload.TheoryVersion != nil && normalizedPayload.TheoryVersion == nil {
			return nil, infra.NewError("THEORY_VERSION_MISSING", "subjectProfile.analysisPayloads.theoryVersion must not be blank when provided.", http.StatusBadRequest)
		}
		if err := builder.ValidateWeightedPayloadEnvelope("subjectProfile.analysisPayloads.payload", normalizedPayload.Payload); err != nil {
			return nil, err
		}
		normalized.AnalysisPayloads = append(normalized.AnalysisPayloads, normalizedPayload)
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

func cloneAnalysisPayloadValueMap(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = cloneAnalysisPayloadValue(value)
	}
	return cloned
}

func cloneAnalysisPayloadValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnalysisPayloadValueMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAnalysisPayloadValue(item))
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case string:
		return strings.TrimSpace(typed)
	case float64, bool, nil, int, int32, int64:
		return typed
	default:
		return fmt.Sprint(typed)
	}
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
