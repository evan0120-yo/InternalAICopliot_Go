package output

import "com.citrus.internalaicopilot/internal/infra"

// RenderUseCase is the builder-facing output entrypoint.
// It intentionally stays thin today so output orchestration can grow without breaking callers.
type RenderUseCase struct {
	service *RenderService
}

// NewRenderUseCase builds the output entrypoint.
func NewRenderUseCase(service *RenderService) *RenderUseCase {
	return &RenderUseCase{service: service}
}

// Render returns the final frontend-facing consult response.
func (u *RenderUseCase) Render(command RenderCommand) (infra.ConsultBusinessResponse, error) {
	return u.service.Render(command)
}
