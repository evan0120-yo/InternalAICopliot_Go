package output

func renderMarkdown(command RenderCommand) (RenderedFile, error) {
	return RenderedFile{
		FileName:    buildFileName(command.BuilderConfig, "md"),
		ContentType: "text/markdown; charset=utf-8",
		FileBytes:   []byte(command.BusinessResponse.Response),
	}, nil
}
