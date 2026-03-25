package builder

import "context"

// TemplateUseCase is the public admin template entrypoint.
// It intentionally stays thin today so admin orchestration can grow without changing handlers.
type TemplateUseCase struct {
	service *TemplateService
}

// NewTemplateUseCase builds the template entrypoint.
func NewTemplateUseCase(service *TemplateService) *TemplateUseCase {
	return &TemplateUseCase{service: service}
}

// ListTemplatesByBuilder returns builder-specific templates.
func (u *TemplateUseCase) ListTemplatesByBuilder(ctx context.Context, builderID int) ([]BuilderTemplateResponse, error) {
	return u.service.ListTemplatesByBuilder(ctx, builderID)
}

// ListAllTemplates returns the full template library.
func (u *TemplateUseCase) ListAllTemplates(ctx context.Context) ([]BuilderTemplateResponse, error) {
	return u.service.ListAllTemplates(ctx)
}

// CreateTemplate creates a new template.
func (u *TemplateUseCase) CreateTemplate(ctx context.Context, request BuilderTemplateRequest) (BuilderTemplateResponse, error) {
	return u.service.CreateTemplate(ctx, request)
}

// UpdateTemplate updates an existing template.
func (u *TemplateUseCase) UpdateTemplate(ctx context.Context, templateID int64, request BuilderTemplateRequest) (BuilderTemplateResponse, error) {
	return u.service.UpdateTemplate(ctx, templateID, request)
}

// DeleteTemplate removes a template.
func (u *TemplateUseCase) DeleteTemplate(ctx context.Context, templateID int64) error {
	return u.service.DeleteTemplate(ctx, templateID)
}
