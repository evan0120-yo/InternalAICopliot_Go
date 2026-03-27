package infra

import "strings"

// OutputFormat is the supported final output type.
type OutputFormat string

const (
	OutputFormatMarkdown OutputFormat = "markdown"
	OutputFormatXLSX     OutputFormat = "xlsx"
)

// ParseOutputFormat validates frontend output format input.
func ParseOutputFormat(raw string) (OutputFormat, bool) {
	switch OutputFormat(strings.ToLower(strings.TrimSpace(raw))) {
	case OutputFormatMarkdown:
		return OutputFormatMarkdown, true
	case OutputFormatXLSX:
		return OutputFormatXLSX, true
	default:
		return "", false
	}
}

// ConsultFilePayload is the optional rendered file payload.
type ConsultFilePayload struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Base64      string `json:"base64"`
}

// ConsultBusinessResponse matches the frontend contract.
type ConsultBusinessResponse struct {
	Status    bool                `json:"status"`
	StatusAns string              `json:"statusAns"`
	Response  string              `json:"response"`
	File      *ConsultFilePayload `json:"file,omitempty"`
	Preview   bool                `json:"-"`
}

// Attachment represents a single consult upload.
type Attachment struct {
	FileName    string
	ContentType string
	Data        []byte
}

// AppAccess is the persisted external app authorization record.
type AppAccess struct {
	AppID                string   `json:"appId" firestore:"appId"`
	Name                 string   `json:"name" firestore:"name"`
	Description          string   `json:"description" firestore:"description"`
	Active               bool     `json:"active" firestore:"active"`
	AllowedBuilderIDs    []int    `json:"allowedBuilderIds" firestore:"allowedBuilderIds"`
	ServiceAccountEmails []string `json:"serviceAccountEmails,omitempty" firestore:"serviceAccountEmails,omitempty"`
}

// AppPromptConfig is the app-aware prompt strategy registry record.
type AppPromptConfig struct {
	AppID       string `json:"appId" firestore:"appId"`
	StrategyKey string `json:"strategyKey" firestore:"strategyKey"`
	Active      bool   `json:"active" firestore:"active"`
}

// BuilderConfig is the persisted builder aggregate root.
type BuilderConfig struct {
	BuilderID           int     `json:"builderId" firestore:"builderId"`
	BuilderCode         string  `json:"builderCode" firestore:"builderCode"`
	GroupKey            *string `json:"groupKey,omitempty" firestore:"groupKey,omitempty"`
	GroupLabel          string  `json:"groupLabel" firestore:"groupLabel"`
	Name                string  `json:"name" firestore:"name"`
	Description         string  `json:"description" firestore:"description"`
	IncludeFile         bool    `json:"includeFile" firestore:"includeFile"`
	DefaultOutputFormat *string `json:"defaultOutputFormat,omitempty" firestore:"defaultOutputFormat,omitempty"`
	FilePrefix          string  `json:"filePrefix" firestore:"filePrefix"`
	Active              bool    `json:"active" firestore:"active"`
}

// Source is a persisted builder source block.
type Source struct {
	SourceID                      int64    `json:"sourceId" firestore:"sourceId"`
	BuilderID                     int      `json:"builderId" firestore:"builderId"`
	CopiedFromTemplateID          *int64   `json:"copiedFromTemplateId,omitempty" firestore:"copiedFromTemplateId,omitempty"`
	CopiedFromTemplateKey         *string  `json:"copiedFromTemplateKey,omitempty" firestore:"copiedFromTemplateKey,omitempty"`
	CopiedFromTemplateName        *string  `json:"copiedFromTemplateName,omitempty" firestore:"copiedFromTemplateName,omitempty"`
	CopiedFromTemplateDescription *string  `json:"copiedFromTemplateDescription,omitempty" firestore:"copiedFromTemplateDescription,omitempty"`
	CopiedFromTemplateGroupKey    *string  `json:"copiedFromTemplateGroupKey,omitempty" firestore:"copiedFromTemplateGroupKey,omitempty"`
	ModuleKey                     string   `json:"moduleKey,omitempty" firestore:"moduleKey,omitempty"`
	SourceType                    string   `json:"sourceType,omitempty" firestore:"sourceType,omitempty"`
	MatchKey                      string   `json:"matchKey,omitempty" firestore:"matchKey,omitempty"`
	Tags                          []string `json:"tags,omitempty" firestore:"tags,omitempty"`
	SourceIDs                     []int64  `json:"sourceIds,omitempty" firestore:"sourceIds,omitempty"`
	Prompts                       string   `json:"prompts" firestore:"prompts"`
	OrderNo                       int      `json:"orderNo" firestore:"orderNo"`
	SystemBlock                   bool     `json:"systemBlock" firestore:"systemBlock"`
	NeedsRagSupplement            bool     `json:"needsRagSupplement" firestore:"needsRagSupplement"`
}

// RagSupplement is a persisted source-level rag config.
type RagSupplement struct {
	RagID         int64  `json:"ragId" firestore:"ragId"`
	SourceID      int64  `json:"sourceId" firestore:"sourceId"`
	RagType       string `json:"ragType" firestore:"ragType"`
	Title         string `json:"title" firestore:"title"`
	Content       string `json:"content" firestore:"content"`
	OrderNo       int    `json:"orderNo" firestore:"orderNo"`
	Overridable   bool   `json:"overridable" firestore:"overridable"`
	RetrievalMode string `json:"retrievalMode" firestore:"retrievalMode"`
}

// Template is the persisted reusable source template.
type Template struct {
	TemplateID  int64   `json:"templateId" firestore:"templateId"`
	TemplateKey string  `json:"templateKey" firestore:"templateKey"`
	Name        string  `json:"name" firestore:"name"`
	Description string  `json:"description" firestore:"description"`
	GroupKey    *string `json:"groupKey,omitempty" firestore:"groupKey,omitempty"`
	OrderNo     int     `json:"orderNo" firestore:"orderNo"`
	Prompts     string  `json:"prompts" firestore:"prompts"`
	Active      bool    `json:"active" firestore:"active"`
}

// TemplateRag is a persisted reusable template rag config.
type TemplateRag struct {
	TemplateRagID int64  `json:"templateRagId" firestore:"templateRagId"`
	TemplateID    int64  `json:"templateId" firestore:"templateId"`
	RagType       string `json:"ragType" firestore:"ragType"`
	Title         string `json:"title" firestore:"title"`
	Content       string `json:"content" firestore:"content"`
	OrderNo       int    `json:"orderNo" firestore:"orderNo"`
	Overridable   bool   `json:"overridable" firestore:"overridable"`
	RetrievalMode string `json:"retrievalMode" firestore:"retrievalMode"`
}

const (
	SourceTypePrimary  = "primary"
	SourceTypeFragment = "fragment"
)
