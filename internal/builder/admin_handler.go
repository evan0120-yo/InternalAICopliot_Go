package builder

import (
	"net/http"
	"strconv"

	"com.citrus.internalaicopilot/internal/infra"
)

// AdminHandler exposes graph/template admin APIs.
type AdminHandler struct {
	graphUseCase    *GraphUseCase
	templateUseCase *TemplateUseCase
}

// NewAdminHandler builds the admin HTTP handler.
func NewAdminHandler(graphUseCase *GraphUseCase, templateUseCase *TemplateUseCase) *AdminHandler {
	return &AdminHandler{
		graphUseCase:    graphUseCase,
		templateUseCase: templateUseCase,
	}
}

// Register registers builder admin routes.
func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/builders/{builderId}/graph", h.loadGraph)
	mux.HandleFunc("PUT /api/admin/builders/{builderId}/graph", h.saveGraph)
	mux.HandleFunc("GET /api/admin/builders/{builderId}/templates", h.listBuilderTemplates)
	mux.HandleFunc("GET /api/admin/templates", h.listAllTemplates)
	mux.HandleFunc("POST /api/admin/templates", h.createTemplate)
	mux.HandleFunc("PUT /api/admin/templates/{templateId}", h.updateTemplate)
	mux.HandleFunc("DELETE /api/admin/templates/{templateId}", h.deleteTemplate)
}

func (h *AdminHandler) loadGraph(w http.ResponseWriter, r *http.Request) {
	builderID, err := parseIntPathValue(r, "builderId", "BUILDER_ID_MISSING")
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	response, err := h.graphUseCase.LoadGraph(r.Context(), builderID)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *AdminHandler) saveGraph(w http.ResponseWriter, r *http.Request) {
	builderID, err := parseIntPathValue(r, "builderId", "BUILDER_ID_MISSING")
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	var request BuilderGraphRequest
	if err := infra.DecodeJSONStrict(w, r, &request, infra.DefaultJSONBodyLimitBytes); err != nil {
		infra.WriteError(w, err)
		return
	}
	response, err := h.graphUseCase.SaveGraph(r.Context(), builderID, request)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *AdminHandler) listBuilderTemplates(w http.ResponseWriter, r *http.Request) {
	builderID, err := parseIntPathValue(r, "builderId", "BUILDER_ID_MISSING")
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	response, err := h.templateUseCase.ListTemplatesByBuilder(r.Context(), builderID)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *AdminHandler) listAllTemplates(w http.ResponseWriter, r *http.Request) {
	response, err := h.templateUseCase.ListAllTemplates(r.Context())
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *AdminHandler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var request BuilderTemplateRequest
	if err := infra.DecodeJSONStrict(w, r, &request, infra.DefaultJSONBodyLimitBytes); err != nil {
		infra.WriteError(w, err)
		return
	}
	response, err := h.templateUseCase.CreateTemplate(r.Context(), request)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusCreated, response)
}

func (h *AdminHandler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := parseInt64PathValue(r, "templateId", "TEMPLATE_ID_MISSING")
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	var request BuilderTemplateRequest
	if err := infra.DecodeJSONStrict(w, r, &request, infra.DefaultJSONBodyLimitBytes); err != nil {
		infra.WriteError(w, err)
		return
	}
	response, err := h.templateUseCase.UpdateTemplate(r.Context(), templateID, request)
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, response)
}

func (h *AdminHandler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := parseInt64PathValue(r, "templateId", "TEMPLATE_ID_MISSING")
	if err != nil {
		infra.WriteError(w, err)
		return
	}
	if err := h.templateUseCase.DeleteTemplate(r.Context(), templateID); err != nil {
		infra.WriteError(w, err)
		return
	}
	infra.WriteJSON(w, http.StatusOK, nil)
}

func parseIntPathValue(r *http.Request, key, code string) (int, error) {
	value := r.PathValue(key)
	if value == "" {
		return 0, infra.NewError(code, key+" is required.", 400)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, infra.NewError(code, key+" must be numeric.", 400)
	}
	return parsed, nil
}

func parseInt64PathValue(r *http.Request, key, code string) (int64, error) {
	value := r.PathValue(key)
	if value == "" {
		return 0, infra.NewError(code, key+" is required.", 400)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, infra.NewError(code, key+" must be numeric.", 400)
	}
	return parsed, nil
}
