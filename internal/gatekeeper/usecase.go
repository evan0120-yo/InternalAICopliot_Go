package gatekeeper

import (
	"context"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
)

// UseCase is the gatekeeper orchestration entrypoint.
type UseCase struct {
	guardService   *GuardService
	builderQuery   *builder.QueryService
	builderConsult *builder.ConsultUseCase
}

// NewUseCase builds the gatekeeper entrypoint.
func NewUseCase(guardService *GuardService, builderQuery *builder.QueryService, builderConsult *builder.ConsultUseCase) *UseCase {
	return &UseCase{
		guardService:   guardService,
		builderQuery:   builderQuery,
		builderConsult: builderConsult,
	}
}

// GuardService exposes the internal guard service for HTTP adaptation.
func (u *UseCase) GuardService() *GuardService {
	return u.guardService
}

// ListBuilders returns active builders for the frontend.
func (u *UseCase) ListBuilders(ctx context.Context) ([]builder.BuilderSummary, error) {
	return u.builderQuery.ListActiveBuilders(ctx)
}

// ListExternalBuilders returns the active builders available to one external app.
func (u *UseCase) ListExternalBuilders(ctx context.Context, appID string) ([]builder.BuilderSummary, error) {
	app, err := u.guardService.ValidateExternalApp(ctx, appID)
	if err != nil {
		return nil, err
	}

	builders, err := u.builderQuery.ListActiveBuilders(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]builder.BuilderSummary, 0, len(builders))
	for _, item := range builders {
		if appAllowsBuilder(app, item.BuilderID) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// Consult validates and forwards a consult request.
func (u *UseCase) Consult(ctx context.Context, appID string, builderID int, text, outputFormat string, attachments []infra.Attachment, clientIP string) (infra.ConsultBusinessResponse, error) {
	builderConfig, parsedFormat, err := u.guardService.ValidateConsult(ctx, builderID, outputFormat, attachments, clientIP)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeGeneric,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		Text:             text,
		OutputFormat:     parsedFormat,
		Attachments:      attachments,
		ClientIP:         clientIP,
	})
}

// PublicProfileConsult validates and forwards a public structured profile consult request.
// appID is treated as an optional prompt-strategy hint and does not trigger external app authorization.
func (u *UseCase) PublicProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, text string, mode infra.AIExecutionMode, clientIP string) (infra.ConsultBusinessResponse, error) {
	builderConfig, normalizedProfile, err := u.guardService.ValidateProfileConsult(ctx, appID, builderID, subjectProfile, text, clientIP)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeProfile,
		AIExecutionMode:  mode,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		Text:             text,
		ClientIP:         clientIP,
		SubjectProfile:   normalizedProfile,
	})
}

// ExternalConsult validates and forwards an external app consult request.
func (u *UseCase) ExternalConsult(ctx context.Context, appID string, builderID int, text, outputFormat string, attachments []infra.Attachment, clientIP string) (infra.ConsultBusinessResponse, error) {
	_, builderConfig, parsedFormat, err := u.guardService.ValidateExternalConsult(ctx, appID, builderID, outputFormat, attachments, clientIP)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeGeneric,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		Text:             text,
		OutputFormat:     parsedFormat,
		Attachments:      attachments,
		ClientIP:         clientIP,
	})
}

// ProfileConsult validates and forwards a structured profile consult request.
func (u *UseCase) ProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, text, clientIP string) (infra.ConsultBusinessResponse, error) {
	var (
		builderConfig     infra.BuilderConfig
		normalizedProfile *builder.SubjectProfile
		err               error
	)
	if appID == "" {
		builderConfig, normalizedProfile, err = u.guardService.ValidateProfileConsult(ctx, appID, builderID, subjectProfile, text, clientIP)
	} else {
		_, builderConfig, normalizedProfile, err = u.guardService.ValidateExternalProfileConsult(ctx, appID, builderID, subjectProfile, text, clientIP)
	}
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeProfile,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		Text:             text,
		ClientIP:         clientIP,
		SubjectProfile:   normalizedProfile,
	})
}
