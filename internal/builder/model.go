package builder

import "com.citrus.internalaicopilot/internal/infra"

// ConsultMode keeps generic consult and profile consult paths explicit.
type ConsultMode int

const (
	ConsultModeGeneric ConsultMode = iota
	ConsultModeProfile
)

// BuilderSummary matches the frontend builder dropdown contract.
type BuilderSummary struct {
	BuilderID           int     `json:"builderId"`
	BuilderCode         string  `json:"builderCode"`
	GroupKey            *string `json:"groupKey,omitempty"`
	GroupLabel          string  `json:"groupLabel"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	IncludeFile         bool    `json:"includeFile"`
	DefaultOutputFormat *string `json:"defaultOutputFormat,omitempty"`
}

// ConsultCommand is the builder consult entry input.
type ConsultCommand struct {
	Mode             ConsultMode
	AppID            string
	BuilderID        int
	PreloadedBuilder *infra.BuilderConfig
	Text             string
	OutputFormat     *infra.OutputFormat
	Attachments      []infra.Attachment
	ClientIP         string
	SubjectProfile   *SubjectProfile
}

type promptAssemblyResult struct {
	Instructions    string
	UserMessageText string
}

// SubjectProfile is the structured profile-analysis payload.
type SubjectProfile struct {
	SubjectID        string
	AnalysisPayloads []SubjectAnalysisPayload
}

// SubjectAnalysisPayload is one analysis-type-specific profile payload.
type SubjectAnalysisPayload struct {
	AnalysisType  string
	TheoryVersion *string
	Payload       map[string]any
}

// BuilderGraphBuilderResponse is the builder graph builder payload.
type BuilderGraphBuilderResponse struct {
	BuilderID           int     `json:"builderId"`
	BuilderCode         string  `json:"builderCode"`
	GroupKey            *string `json:"groupKey,omitempty"`
	GroupLabel          string  `json:"groupLabel"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	IncludeFile         bool    `json:"includeFile"`
	DefaultOutputFormat *string `json:"defaultOutputFormat,omitempty"`
	FilePrefix          string  `json:"filePrefix"`
	Active              bool    `json:"active"`
}

// BuilderGraphRagResponse is the graph rag payload.
type BuilderGraphRagResponse struct {
	RagID         int64  `json:"ragId"`
	RagType       string `json:"ragType"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	OrderNo       int    `json:"orderNo"`
	Overridable   bool   `json:"overridable"`
	RetrievalMode string `json:"retrievalMode"`
}

// BuilderGraphSourceResponse is the graph source payload.
type BuilderGraphSourceResponse struct {
	SourceID            int64                     `json:"sourceId"`
	TemplateID          *int64                    `json:"templateId,omitempty"`
	TemplateKey         *string                   `json:"templateKey,omitempty"`
	TemplateName        *string                   `json:"templateName,omitempty"`
	TemplateDescription *string                   `json:"templateDescription,omitempty"`
	TemplateGroupKey    *string                   `json:"templateGroupKey,omitempty"`
	ModuleKey           *string                   `json:"moduleKey,omitempty"`
	OrderNo             int                       `json:"orderNo"`
	SystemBlock         bool                      `json:"systemBlock"`
	Prompts             string                    `json:"prompts"`
	Rag                 []BuilderGraphRagResponse `json:"rag"`
}

// BuilderGraphResponse is the admin graph response.
type BuilderGraphResponse struct {
	Builder BuilderGraphBuilderResponse  `json:"builder"`
	Sources []BuilderGraphSourceResponse `json:"sources"`
}

// BuilderGraphBuilderRequest is the admin graph builder request.
type BuilderGraphBuilderRequest struct {
	BuilderCode         *string `json:"builderCode"`
	GroupKey            *string `json:"groupKey"`
	GroupLabel          *string `json:"groupLabel"`
	Name                *string `json:"name"`
	Description         *string `json:"description"`
	IncludeFile         *bool   `json:"includeFile"`
	DefaultOutputFormat *string `json:"defaultOutputFormat"`
	FilePrefix          *string `json:"filePrefix"`
	Active              *bool   `json:"active"`
}

// BuilderGraphRagRequest is the admin graph rag request.
type BuilderGraphRagRequest struct {
	RagType       *string `json:"ragType"`
	Title         *string `json:"title"`
	Content       string  `json:"content"`
	Prompts       string  `json:"prompts"`
	OrderNo       *int    `json:"orderNo"`
	Overridable   *bool   `json:"overridable"`
	RetrievalMode *string `json:"retrievalMode"`
}

// BuilderGraphSourceRequest is the admin graph source request.
type BuilderGraphSourceRequest struct {
	TemplateID          *int64                   `json:"templateId"`
	TemplateKey         *string                  `json:"templateKey"`
	TemplateName        *string                  `json:"templateName"`
	TemplateDescription *string                  `json:"templateDescription"`
	TemplateGroupKey    *string                  `json:"templateGroupKey"`
	ModuleKey           *string                  `json:"moduleKey"`
	OrderNo             *int                     `json:"orderNo"`
	SystemBlock         *bool                    `json:"systemBlock"`
	Prompts             string                   `json:"prompts"`
	Rag                 []BuilderGraphRagRequest `json:"rag"`
}

// BuilderGraphAiAgentItemRequest supports the legacy payload shape kept in Java.
type BuilderGraphAiAgentItemRequest struct {
	Source *BuilderGraphSourceRequest `json:"source"`
}

// BuilderGraphRequest is the admin graph save request.
type BuilderGraphRequest struct {
	Builder *BuilderGraphBuilderRequest      `json:"builder"`
	Sources []BuilderGraphSourceRequest      `json:"sources"`
	AiAgent []BuilderGraphAiAgentItemRequest `json:"aiagent"`
}

// BuilderTemplateRagResponse is the template rag response.
type BuilderTemplateRagResponse struct {
	TemplateRagID int64  `json:"templateRagId"`
	RagType       string `json:"ragType"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	OrderNo       int    `json:"orderNo"`
	Overridable   bool   `json:"overridable"`
	RetrievalMode string `json:"retrievalMode"`
}

// BuilderTemplateResponse is the template response.
type BuilderTemplateResponse struct {
	TemplateID  int64                        `json:"templateId"`
	TemplateKey string                       `json:"templateKey"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	GroupKey    *string                      `json:"groupKey,omitempty"`
	OrderNo     int                          `json:"orderNo"`
	Prompts     string                       `json:"prompts"`
	Active      bool                         `json:"active"`
	Rag         []BuilderTemplateRagResponse `json:"rag"`
}

// BuilderTemplateRagRequest is the template rag request.
type BuilderTemplateRagRequest struct {
	RagType       *string `json:"ragType"`
	Title         *string `json:"title"`
	Content       string  `json:"content"`
	OrderNo       *int    `json:"orderNo"`
	Overridable   *bool   `json:"overridable"`
	RetrievalMode *string `json:"retrievalMode"`
}

// BuilderTemplateRequest is the template save request.
type BuilderTemplateRequest struct {
	TemplateKey *string                     `json:"templateKey"`
	Name        *string                     `json:"name"`
	Description *string                     `json:"description"`
	GroupKey    *string                     `json:"groupKey"`
	OrderNo     *int                        `json:"orderNo"`
	Prompts     *string                     `json:"prompts"`
	Active      *bool                       `json:"active"`
	Rag         []BuilderTemplateRagRequest `json:"rag"`
}
