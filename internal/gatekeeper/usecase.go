package gatekeeper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/promptguard"
)

// UseCase is the gatekeeper orchestration entrypoint.
type UseCase struct {
	guardService       *GuardService
	promptGuardUseCase *promptguard.EvaluateUseCase
	builderQuery       *builder.QueryService
	builderConsult     *builder.ConsultUseCase
}

var lineTaskNow = time.Now

// NewUseCase builds the gatekeeper entrypoint.
func NewUseCase(guardService *GuardService, promptGuardUseCase *promptguard.EvaluateUseCase, builderQuery *builder.QueryService, builderConsult *builder.ConsultUseCase) *UseCase {
	return &UseCase{
		guardService:       guardService,
		promptGuardUseCase: promptGuardUseCase,
		builderQuery:       builderQuery,
		builderConsult:     builderConsult,
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
	log.Printf("gatekeeper consult: builderID=%d appID=%q attachments=%d clientIP=%q", builderID, appID, len(attachments), clientIP)
	builderConfig, parsedFormat, err := u.guardService.ValidateConsult(ctx, builderID, outputFormat, attachments, clientIP)
	if err != nil {
		log.Printf("gatekeeper consult validate failed: builderID=%d err=%v", builderID, err)
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
func (u *UseCase) PublicProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, userText, intentText string, mode infra.AIExecutionMode, clientIP string) (infra.ConsultBusinessResponse, error) {
	userText = strings.TrimSpace(userText)
	intentText = strings.TrimSpace(intentText)

	builderConfig, normalizedProfile, err := u.guardService.ValidateProfileConsult(ctx, appID, builderID, subjectProfile, userText, intentText, clientIP)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}
	if blockedResponse, err := u.evaluatePromptGuard(ctx, appID, builderConfig, userText, intentText); err != nil || blockedResponse != nil {
		if err != nil {
			return infra.ConsultBusinessResponse{}, err
		}
		return *blockedResponse, nil
	}

	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeProfile,
		AIExecutionMode:  mode,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		UserText:         userText,
		IntentText:       intentText,
		ClientIP:         clientIP,
		SubjectProfile:   normalizedProfile,
	})
}

// ExternalConsult validates and forwards an external app consult request.
func (u *UseCase) ExternalConsult(ctx context.Context, appID string, builderID int, text, outputFormat string, attachments []infra.Attachment, clientIP string) (infra.ConsultBusinessResponse, error) {
	log.Printf("gatekeeper external consult: builderID=%d appID=%q attachments=%d clientIP=%q", builderID, appID, len(attachments), clientIP)
	_, builderConfig, parsedFormat, err := u.guardService.ValidateExternalConsult(ctx, appID, builderID, outputFormat, attachments, clientIP)
	if err != nil {
		log.Printf("gatekeeper external consult validate failed: builderID=%d appID=%q err=%v", builderID, appID, err)
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
func (u *UseCase) ProfileConsult(ctx context.Context, appID string, builderID int, subjectProfile *builder.SubjectProfile, userText, intentText, clientIP string) (infra.ConsultBusinessResponse, error) {
	userText = strings.TrimSpace(userText)
	intentText = strings.TrimSpace(intentText)
	subjectID := ""
	if subjectProfile != nil {
		subjectID = subjectProfile.SubjectID
	}
	log.Printf("gatekeeper profile consult: builderID=%d appID=%q subjectID=%q clientIP=%q", builderID, appID, subjectID, clientIP)

	var (
		builderConfig     infra.BuilderConfig
		normalizedProfile *builder.SubjectProfile
		err               error
	)
	if appID == "" {
		builderConfig, normalizedProfile, err = u.guardService.ValidateProfileConsult(ctx, appID, builderID, subjectProfile, userText, intentText, clientIP)
	} else {
		_, builderConfig, normalizedProfile, err = u.guardService.ValidateExternalProfileConsult(ctx, appID, builderID, subjectProfile, userText, intentText, clientIP)
	}
	if err != nil {
		log.Printf("gatekeeper profile consult validate failed: builderID=%d appID=%q err=%v", builderID, appID, err)
		return infra.ConsultBusinessResponse{}, err
	}
	if blockedResponse, err := u.evaluatePromptGuard(ctx, appID, builderConfig, userText, intentText); err != nil || blockedResponse != nil {
		if err != nil {
			return infra.ConsultBusinessResponse{}, err
		}
		return *blockedResponse, nil
	}

	return u.builderConsult.Consult(ctx, builder.ConsultCommand{
		Mode:             builder.ConsultModeProfile,
		AppID:            appID,
		BuilderID:        builderID,
		PreloadedBuilder: &builderConfig,
		UserText:         userText,
		IntentText:       intentText,
		ClientIP:         clientIP,
		SubjectProfile:   normalizedProfile,
	})
}

// PublicLineTaskConsult validates and forwards a local/dev LineTask extraction request.
// appID is treated as an optional builder context hint and does not trigger external app authorization.
func (u *UseCase) PublicLineTaskConsult(ctx context.Context, appID string, builderID int, messageText, referenceTime, timeZone string, supportedTaskTypes []string, clientIP string) (infra.ConsultBusinessResponse, error) {
	builderConfig, err := u.guardService.ValidateLineTaskConsult(ctx, builderID, messageText, clientIP)
	if err != nil {
		return infra.ConsultBusinessResponse{}, err
	}

	return u.builderConsult.Consult(ctx, buildLineTaskCommand(appID, builderID, messageText, referenceTime, timeZone, supportedTaskTypes, clientIP, builderConfig))
}

// LineTaskConsult validates and forwards a LineBot extraction request.
func (u *UseCase) LineTaskConsult(ctx context.Context, appID string, builderID int, messageText, referenceTime, timeZone string, supportedTaskTypes []string, clientIP string) (infra.ConsultBusinessResponse, error) {
	log.Printf("gatekeeper line-task consult: builderID=%d appID=%q supportedTaskTypes=%v clientIP=%q", builderID, appID, supportedTaskTypes, clientIP)
	_, builderConfig, err := u.guardService.ValidateExternalLineTaskConsult(ctx, appID, builderID, messageText, clientIP)
	if err != nil {
		log.Printf("gatekeeper line-task consult validate failed: builderID=%d appID=%q err=%v", builderID, appID, err)
		return infra.ConsultBusinessResponse{}, err
	}

	return u.builderConsult.Consult(ctx, buildLineTaskCommand(appID, builderID, messageText, referenceTime, timeZone, supportedTaskTypes, clientIP, builderConfig))
}

func (u *UseCase) evaluatePromptGuard(ctx context.Context, appID string, builderConfig infra.BuilderConfig, userText, intentText string) (*infra.ConsultBusinessResponse, error) {
	if u.promptGuardUseCase == nil || !shouldRunPromptGuard(builderConfig, userText, intentText) {
		return nil, nil
	}

	for _, candidate := range []string{strings.TrimSpace(userText), strings.TrimSpace(intentText)} {
		if candidate == "" {
			continue
		}

		evaluation, err := u.promptGuardUseCase.Evaluate(ctx, promptguard.Command{
			AppID:         appID,
			BuilderConfig: builderConfig,
			UserText:      candidate,
		})
		if err != nil {
			return nil, err
		}

		switch evaluation.Decision {
		case promptguard.DecisionAllow:
			continue
		case promptguard.DecisionBlock:
			responseDetail := strings.TrimSpace(evaluation.Reason)
			if responseDetail == "" {
				responseDetail = "promptguard blocked request"
			}
			response := infra.ConsultBusinessResponse{
				Status:         false,
				StatusAns:      "prompts有違法注入內容",
				Response:       "取消回應",
				ResponseDetail: responseDetail,
			}
			return &response, nil
		default:
			return nil, infra.NewError("PROMPTGUARD_DECISION_UNRESOLVED", "Promptguard did not return a final allow/block decision.", 500)
		}
	}
	return nil, nil
}

func shouldRunPromptGuard(builderConfig infra.BuilderConfig, userText, intentText string) bool {
	if strings.TrimSpace(userText) == "" && strings.TrimSpace(intentText) == "" {
		return false
	}
	return strings.TrimSpace(builderConfig.BuilderCode) == "linkchat-astrology"
}

func buildLineTaskCommand(appID string, builderID int, messageText, referenceTime, timeZone string, supportedTaskTypes []string, clientIP string, builderConfig infra.BuilderConfig) builder.ConsultCommand {
	resolvedReferenceTime, resolvedTimeZone := resolveLineTaskExecutionContext(referenceTime, timeZone)

	return builder.ConsultCommand{
		Mode:               builder.ConsultModeExtract,
		AIExecutionMode:    infra.AIExecutionModeLive,
		AppID:              strings.TrimSpace(appID),
		BuilderID:          builderID,
		PreloadedBuilder:   &builderConfig,
		Text:               strings.TrimSpace(messageText),
		ReferenceTime:      resolvedReferenceTime,
		TimeZone:           resolvedTimeZone,
		SupportedTaskTypes: normalizeLineTaskSupportedTaskTypes(supportedTaskTypes),
		ClientIP:           clientIP,
	}
}

func normalizeLineTaskSupportedTaskTypes(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return []string{"calendar"}
	}
	return normalized
}

func resolveLineTaskExecutionContext(referenceTime, timeZone string) (string, string) {
	now := lineTaskNow()

	trimmedReferenceTime := strings.TrimSpace(referenceTime)
	if trimmedReferenceTime == "" {
		trimmedReferenceTime = now.Format("2006-01-02 15:04:05")
	}

	trimmedTimeZone := strings.TrimSpace(timeZone)
	if trimmedTimeZone == "" {
		trimmedTimeZone = defaultLineTaskTimeZone(now)
	}

	return trimmedReferenceTime, trimmedTimeZone
}

func defaultLineTaskTimeZone(now time.Time) string {
	locationName := strings.TrimSpace(now.Location().String())
	if locationName != "" && !strings.EqualFold(locationName, "local") {
		return locationName
	}

	zoneName, offsetSeconds := now.Zone()
	zoneName = strings.TrimSpace(zoneName)
	if zoneName != "" && !strings.EqualFold(zoneName, "local") {
		return zoneName
	}

	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}
