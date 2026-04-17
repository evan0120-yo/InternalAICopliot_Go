# Internal AI Copilot Go Backend — SDD Contracts

## External App Contract (X-App-Id / gRPC appId)

```text
request
├─ appId          string  required for external HTTP + external gRPC
├─ allowedBuilderIds  []string  must contain target builderId
└─ active         bool    app record must be active

rules
├─ missing appId → reject
├─ appId not found or inactive → reject
└─ builder not in allowedBuilderIds → reject
```

## Consult Request Contract

```text
shared fields
├─ builderId      string   required
├─ userText       string   optional
├─ outputFormat   string   optional; must be valid enum if provided
└─ attachments    []file   optional; validated before AI call

attachment rules
├─ count limit    enforced per builder config
├─ size limit     enforced per builder config
└─ ext whitelist  enforced per builder config

profile consult additional fields
├─ subjectProfile object   normalized before consult
└─ intentText     string   optional; triggers promptguard if non-empty

line task additional fields
├─ messageText    string   required
├─ referenceTime  string   YYYY-MM-DD HH:mm:ss; auto-filled from backend time if empty
├─ timeZone       string   auto-filled from location fallback UTC±HH:MM if empty
└─ supportedTaskTypes []string  default ["calendar"] if empty
```

## Consult Response Contract

```text
business response
├─ success → text response [+ file payload if builder requires file]
├─ block   → business response with block reason (not HTTP 4xx)
└─ non-success → no file payload

line task structured result
├─ taskType
├─ operation
├─ summary
├─ startAt / endAt
├─ location
├─ missingFields
├─ eventId
├─ queryStartAt / queryEndAt
```

## Admin Graph Contract

```text
load response
├─ builder info
├─ sources []
└─ source rags []   (per source)

save rules
├─ non-systemBlock sources → wholesale replace (delete old, write new)
└─ systemBlock sources     → never overwritten by request payload
```

## Firestore Collections

```text
apps/{appId}
appPromptConfigs/{appId}
builders/{builderId}
builders/{builderId}/sources/{sourceId}
builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}
templates/{templateId}
templates/{templateId}/templateRags/{templateRagId}
_meta/counters
_sourceLookup/{sourceId}
```

## Config / Env Vars

```text
FIRESTORE_PROJECT_ID      Firestore project
FIRESTORE_EMULATOR_HOST   emulator address (dev only)
GRPC_PORT                 gRPC listen port
HTTP_PORT                 HTTP listen port
OPENAI_API_KEY            OpenAI provider key
GEMMA_ENDPOINT            Gemma provider URL
AI_EXECUTION_MODE         preview | mock | live
```
