package gatekeeper

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/infra"
)

// Handler exposes the public gatekeeper HTTP routes.
type Handler struct {
	useCase *UseCase
}

// NewHandler builds the gatekeeper HTTP handler.
func NewHandler(useCase *UseCase) *Handler {
	return &Handler{useCase: useCase}
}

// Register registers public API routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/builders", h.listBuilders)
	mux.HandleFunc("POST /api/consult", h.consult)
	mux.HandleFunc("POST /api/profile-consult", h.profileConsult)
	mux.HandleFunc("POST /api/line-task-consult", h.lineTaskConsult)
	mux.HandleFunc("GET /api/external/builders", h.listExternalBuilders)
	mux.HandleFunc("POST /api/external/consult", h.externalConsult)
}

func (h *Handler) listBuilders(w http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.ListBuilders(r.Context())
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) consult(w http.ResponseWriter, r *http.Request) {
	builderID, attachments, ok := parseConsultMultipart(w, r, h.useCase.GuardService().MultipartMemoryLimit())
	if !ok {
		return
	}

	clientIP := h.useCase.GuardService().ResolveClientIP(r)
	response, err := h.useCase.Consult(r.Context(), strings.TrimSpace(r.FormValue("appId")), builderID, r.FormValue("text"), r.FormValue("outputFormat"), attachments, clientIP)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) profileConsult(w http.ResponseWriter, r *http.Request) {
	var request profileConsultRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		infra.WriteError(w, infra.NewError("INVALID_JSON", "Profile consult request must be valid JSON.", http.StatusBadRequest))
		return
	}
	if strings.TrimSpace(request.Mode) != "" {
		if _, ok := infra.ParseAIExecutionMode(request.Mode); !ok {
			infra.WriteError(w, infra.NewError("INVALID_MODE", "Profile consult mode is not supported.", http.StatusBadRequest))
			return
		}
	}

	clientIP := h.useCase.GuardService().ResolveClientIP(r)
	response, err := h.useCase.PublicProfileConsult(
		r.Context(),
		strings.TrimSpace(request.AppID),
		request.BuilderID,
		request.toSubjectProfile(),
		request.effectiveUserText(),
		request.effectiveIntentText(),
		request.executionMode(),
		clientIP,
	)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) lineTaskConsult(w http.ResponseWriter, r *http.Request) {
	var request lineTaskConsultRequest
	if err := infra.DecodeJSONStrict(w, r, &request, 0); err != nil {
		infra.WriteError(w, err)
		return
	}

	clientIP := h.useCase.GuardService().ResolveClientIP(r)
	response, err := h.useCase.PublicLineTaskConsult(
		r.Context(),
		request.AppID,
		request.BuilderID,
		request.MessageText,
		request.ReferenceTime,
		request.TimeZone,
		request.SupportedTaskTypes,
		clientIP,
	)
	if err != nil {
		infra.WriteError(w, err)
		return
	}

	var parsed lineTaskConsultResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Response)), &parsed); err != nil {
		log.Printf(
			"line task response parse failed err=%v raw=%q",
			err,
			previewLineTaskResponse(response.Response, 240),
		)
		infra.WriteError(w, infra.NewError("LINE_TASK_RESPONSE_INVALID", "Line task response did not match the expected JSON contract.", http.StatusBadGateway))
		return
	}
	infra.WriteJSON(w, http.StatusOK, parsed)
}

func (h *Handler) listExternalBuilders(w http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.ListExternalBuilders(r.Context(), r.Header.Get(ExternalAppIDHeader))
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) externalConsult(w http.ResponseWriter, r *http.Request) {
	builderID, attachments, ok := parseConsultMultipart(w, r, h.useCase.GuardService().MultipartMemoryLimit())
	if !ok {
		return
	}

	clientIP := h.useCase.GuardService().ResolveClientIP(r)
	response, err := h.useCase.ExternalConsult(
		r.Context(),
		r.Header.Get(ExternalAppIDHeader),
		builderID,
		r.FormValue("text"),
		r.FormValue("outputFormat"),
		attachments,
		clientIP,
	)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func parseConsultMultipart(w http.ResponseWriter, r *http.Request, multipartMemoryLimit int64) (int, []infra.Attachment, bool) {
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		infra.WriteError(w, infra.NewError("INVALID_MULTIPART", "Consult request must be multipart/form-data.", http.StatusBadRequest))
		return 0, nil, false
	}

	builderID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("builderId")))
	if err != nil {
		infra.WriteError(w, infra.NewError("BUILDER_ID_MISSING", "builderId is required.", http.StatusBadRequest))
		return 0, nil, false
	}

	attachments := make([]infra.Attachment, 0)
	if r.MultipartForm != nil {
		for _, header := range r.MultipartForm.File["files"] {
			file, err := header.Open()
			if err != nil {
				infra.WriteError(w, infra.NewError("FILE_READ_FAILED", "Uploaded file could not be read.", http.StatusBadRequest))
				return 0, nil, false
			}
			bytes, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr != nil {
				infra.WriteError(w, infra.NewError("FILE_READ_FAILED", "Uploaded file could not be read.", http.StatusBadRequest))
				return 0, nil, false
			}
			attachments = append(attachments, infra.Attachment{
				FileName:    header.Filename,
				ContentType: header.Header.Get("Content-Type"),
				Data:        bytes,
			})
		}
	}
	return builderID, attachments, true
}

type profileConsultRequest struct {
	AppID          string                        `json:"appId"`
	BuilderID      int                           `json:"builderId"`
	SubjectProfile *subjectProfileRequestPayload `json:"subjectProfile"`
	UserText       string                        `json:"userText"`
	IntentText     string                        `json:"intentText"`
	Text           string                        `json:"text"`
	Mode           string                        `json:"mode,omitempty"`
}

func (r profileConsultRequest) executionMode() infra.AIExecutionMode {
	mode, _ := infra.ParseAIExecutionMode(r.Mode)
	return mode
}

func (r profileConsultRequest) effectiveUserText() string {
	if strings.TrimSpace(r.UserText) != "" {
		return strings.TrimSpace(r.UserText)
	}
	return strings.TrimSpace(r.Text)
}

func (r profileConsultRequest) effectiveIntentText() string {
	return strings.TrimSpace(r.IntentText)
}

type lineTaskConsultRequest struct {
	AppID              string   `json:"appId"`
	BuilderID          int      `json:"builderId"`
	MessageText        string   `json:"messageText"`
	ReferenceTime      string   `json:"referenceTime"`
	TimeZone           string   `json:"timeZone"`
	SupportedTaskTypes []string `json:"supportedTaskTypes"`
}

type lineTaskConsultResponse struct {
	TaskType      string   `json:"taskType"`
	Operation     string   `json:"operation"`
	EventID       string   `json:"eventId"`
	Summary       string   `json:"summary"`
	StartAt       string   `json:"startAt"`
	EndAt         string   `json:"endAt"`
	QueryStartAt  string   `json:"queryStartAt"`
	QueryEndAt    string   `json:"queryEndAt"`
	Location      string   `json:"location"`
	MissingFields []string `json:"missingFields"`
}

func previewLineTaskResponse(raw string, max int) string {
	trimmed := strings.TrimSpace(raw)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

type subjectProfileRequestPayload struct {
	SubjectID        string                          `json:"subjectId"`
	AnalysisPayloads []subjectAnalysisRequestPayload `json:"analysisPayloads"`
}

type subjectAnalysisRequestPayload struct {
	AnalysisType  string         `json:"analysisType"`
	TheoryVersion *string        `json:"theoryVersion,omitempty"`
	Payload       map[string]any `json:"payload"`
}

func (r profileConsultRequest) toSubjectProfile() *builder.SubjectProfile {
	if r.SubjectProfile == nil {
		return nil
	}

	profile := &builder.SubjectProfile{
		SubjectID:        r.SubjectProfile.SubjectID,
		AnalysisPayloads: make([]builder.SubjectAnalysisPayload, 0, len(r.SubjectProfile.AnalysisPayloads)),
	}
	for _, payload := range r.SubjectProfile.AnalysisPayloads {
		analysisPayload := builder.SubjectAnalysisPayload{
			AnalysisType:  payload.AnalysisType,
			TheoryVersion: cloneOptionalString(payload.TheoryVersion),
			Payload:       clonePayloadMap(payload.Payload),
		}
		profile.AnalysisPayloads = append(profile.AnalysisPayloads, analysisPayload)
	}
	return profile
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func clonePayloadMap(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = clonePayloadValue(value)
	}
	return cloned
}

func clonePayloadValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return clonePayloadMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, clonePayloadValue(item))
		}
		return cloned
	default:
		return typed
	}
}
