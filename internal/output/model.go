package output

import "com.citrus.internalaicopilot/internal/infra"

// RenderedFile is the raw rendered file before base64 encoding.
type RenderedFile struct {
	FileName    string
	ContentType string
	FileBytes   []byte
}

// RenderCommand is the output module input.
type RenderCommand struct {
	BuilderConfig    infra.BuilderConfig
	OutputFormat     *infra.OutputFormat
	BusinessResponse infra.ConsultBusinessResponse
}
