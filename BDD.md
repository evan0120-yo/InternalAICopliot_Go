# Internal AI Copilot Go Backend BDD

## Behavior World

```text
behavior world
├─ entry
│  ├─ public HTTP: GET /api/builders, POST /api/consult
│  ├─ external HTTP: POST /api/profile-consult (with X-App-Id)
│  ├─ local/dev HTTP: POST /api/line-task-consult
│  ├─ gRPC: GenericConsult / ProfileConsult / LineTaskConsult / ListBuilders
│  └─ admin HTTP: graph CRUD, template CRUD
├─ interpretation core
│  └─ builder-driven AI orchestration
│     ├─ generic mode
│     ├─ profile mode
│     └─ extract mode (line task)
├─ operation space
│  ├─ consult (generic / profile / extract)
│  ├─ builder discovery
│  └─ admin (graph / templates)
└─ output space
   ├─ text response
   ├─ file payload (conditional)
   └─ structured typed result (line task)
```

## Hard Behavior Rules

```text
builder contract
├─ must be active to be consulted
├─ inactive builder → request rejected
└─ external app: appId must be valid + active + builder in allowedBuilderIds

consult shared contract
├─ public / external / gRPC: share same builder consult orchestration
├─ outputFormat invalid → reject before AI call
└─ attachments over limit (count / size / ext) → reject before AI call

file output contract
├─ output file: only when business response = success AND builder requires file
└─ non-success response → must not include file payload

promptguard contract
├─ trigger condition: builderCode = linkchat-astrology AND (userText OR intentText non-empty)
├─ block decision → return business response (not 4xx, not empty)
└─ block must not continue to main analysis flow

line task contract
├─ referenceTime empty → auto-fill from backend current time
├─ timeZone empty → auto-fill from location; fallback UTC±HH:MM
├─ supportedTaskTypes empty → default ["calendar"]
├─ local/dev HTTP route → no external app auth required
└─ external gRPC route → must validate appId + allowed builders

graph save contract
├─ non-systemBlock sources → wholesale replace (delete old, write new)
└─ systemBlock source → never overwritten by request payload

admin graph load contract
└─ response must include builder + sources + source rags
```

## Edge Scenarios

### Scenario: promptguard block returns business response

Given builderCode = linkchat-astrology and userText is non-empty  
When promptguard decides to block  
Then response must be a business response (not HTTP 4xx, not error rejection)  
And response must NOT continue into main AI analysis

> Block result looks like a normal response to the caller — it is a structured business response with block reason, not an HTTP error.

### Scenario: line task referenceTime auto-filled when missing

Given external gRPC caller sends LineTaskConsult without referenceTime  
When backend processes the request  
Then backend must use its own current time as concrete referenceTime  
And the AI call must not receive an empty referenceTime
