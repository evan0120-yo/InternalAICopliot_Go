package gatekeeper

import (
	"encoding/json"
	"io"
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

	clientIP := h.useCase.GuardService().ResolveClientIP(r)
	response, err := h.useCase.PublicProfileConsult(
		r.Context(),
		strings.TrimSpace(request.AppID),
		request.BuilderID,
		append([]string(nil), request.AnalysisModules...),
		request.toSubjectProfile(),
		request.Text,
		clientIP,
	)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
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
	AppID           string                        `json:"appId"`
	BuilderID       int                           `json:"builderId"`
	AnalysisModules []string                      `json:"analysisModules"`
	SubjectProfile  *subjectProfileRequestPayload `json:"subjectProfile"`
	Text            string                        `json:"text"`
}

type subjectProfileRequestPayload struct {
	SubjectID      string                        `json:"subjectId"`
	ModulePayloads []subjectModuleRequestPayload `json:"modulePayloads"`
}

type subjectModuleRequestPayload struct {
	ModuleKey     string                   `json:"moduleKey"`
	TheoryVersion *string                  `json:"theoryVersion,omitempty"`
	Facts         []subjectFactRequestItem `json:"facts"`
}

type subjectFactRequestItem struct {
	FactKey string   `json:"factKey"`
	Values  []string `json:"values"`
}

func (r profileConsultRequest) toSubjectProfile() *builder.SubjectProfile {
	if r.SubjectProfile == nil {
		return nil
	}

	profile := &builder.SubjectProfile{
		SubjectID:      r.SubjectProfile.SubjectID,
		ModulePayloads: make([]builder.SubjectModulePayload, 0, len(r.SubjectProfile.ModulePayloads)),
	}
	for _, payload := range r.SubjectProfile.ModulePayloads {
		modulePayload := builder.SubjectModulePayload{
			ModuleKey:     payload.ModuleKey,
			TheoryVersion: cloneOptionalString(payload.TheoryVersion),
			Facts:         make([]builder.SubjectFact, 0, len(payload.Facts)),
		}
		for _, fact := range payload.Facts {
			modulePayload.Facts = append(modulePayload.Facts, builder.SubjectFact{
				FactKey: fact.FactKey,
				Values:  append([]string(nil), fact.Values...),
			})
		}
		profile.ModulePayloads = append(profile.ModulePayloads, modulePayload)
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
