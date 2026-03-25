# Internal AI Copilot Go Backend Plan

## AI Delivery Rule
以下規則在 AI 參與 Go 專案規劃與實作時必須優先遵守：

1. AI 開發不應自行套用人類常見的「權重排序」思維來把需求拆成只做一部分。優先級通常是給人類排程、協作與資源配置使用；當需求已明確交給 AI 執行時，除非有特殊理由，否則需求方預設期待 AI 一次投入時間把事情完整做完。
2. 操作者會希望一次花時間搞定，因為就可以不被 AI 持續打斷去做其他事情。
3. 反之就要一直顧 AI 會非常累。
4. 因此，除非需求方明確要求分批進行、保留後續階段、先做 spike、或實作中出現必須先確認的 blocker，否則 AI 預設應以一次完整交付為原則。

## BDD-First Strict Rule
本專案的 Go 開發與規劃流程採 `BDD-first`，且需嚴格執行。

嚴格執行的意思是：
- 先以行為、情境、驗收條件確認需求，再討論實作
- `PLAN`、開發文件、測試案例、production code 必須以同一組行為規格為基準
- 若文件、測試、實作三者不一致，必須視為缺陷並立即同步修正
- 未經需求與行為規格對齊，不應直接跳到 production code 實作

## Overview
`Go/` 目錄下的 Backend 是 Java 版 Internal AI Copilot 的 Go 重寫版本。

最高原則：
- 對外 public/admin HTTP API 契約盡量和 Java 版一致
- consult / graph / template 的核心業務行為盡量和 Java 版一致
- 資料層從 JPA/PostgreSQL 改為 Firestore
- RAG 架構保留未來向量檢索與外部資料擴充能力
- 開發與規劃流程採 BDD-first，且需嚴格依行為規格同步文件、測試與實作
- 行為規格確認後，以 TDD-first 實作

補充基線：
- Go 版另外承接 external integration 的 gRPC surface
- LinkChat profile-analysis 這條線採單一 builder + dynamic `analysisModules`
- LinkChat profile-analysis 專用入口為 `ProfileConsult`
- LinkChat 在 hot path 不依賴先查 builders discovery，再做 consult

## Delivery Flow
本專案的預設交付順序如下：

1. 先確認 actor、業務目標、成功條件、失敗條件與邊界情境
2. 將需求落成 `PLAN` 與對應開發文件
3. 以 scenario / acceptance criteria 為基準拆成 UseCase 測試與 Service 測試
4. 以最小實作讓測試通過
5. 重構時不得改變既有行為
6. 若實作或測試更新，需同步回寫文件

## Behavior Scope

### Primary actors
- public user：發起 consult 並取得回應
- external profile-analysis app：例如 LinkChat，以固定 `appId` / `builderId` 呼叫 Internal gRPC consult
- external HTTP app：若仍需附件型整合，可使用 external HTTP routes
- admin user：維護 builder graph 與 templates
- AI collaboration agent：先問清行為規格，再依文件與測試實作

### Primary behaviors
- public user 可以查詢可用 builders
- public user 可以對指定 builder 發起 consult
- LinkChat 這類 profile-analysis app 可以送出 structured gRPC profile consult request
- external HTTP app 可以查詢自己被授權的 builders 並發起 consult
- admin user 可以讀取與儲存 builder graph
- admin user 可以查詢、建立、更新、刪除 template
- system 會依 builder/source/rag/module 規則組裝 prompt
- system 會依 output policy 決定是否產出純文字或檔案 payload

## Key Scenarios

### Scenario group: list builders
- 當 public user 查詢 builders 時，只能看到 `active=true` 的 builder
- 回傳結果需維持 Java 契約欄位與排序語意

### Scenario group: consult
- 當 request 合法時，系統應載入 builder/source、resolve RAG、組裝 prompt、呼叫 AI、回傳結果
- 當 `builderId` 不存在或 inactive 時，系統應回傳對應錯誤
- 當 `outputFormat` 非法或附件違反限制時，系統應回傳 validation error
- 當 builder 需要輸出檔案時，系統應依 policy 回傳檔案 payload

### Scenario group: LinkChat profile-analysis integration
- LinkChat 走 dedicated gRPC integration surface，不走每次 request 先查 builders 的 discovery 熱路徑
- LinkChat 以固定 config 傳送 `appId` 與 `builderId`
- `ProfileConsult` request 應顯式帶 `analysisModules[]`
- `ProfileConsult` request 應帶 optional structured `subjectProfile`
- `text` 為 optional 補充輸入，不是 module selection 來源
- Internal 必須驗證 `appId -> builderId`
- builder command 應保留 `appId`，讓下游 prompt assembly 可依 app 選策略
- builder command 必須帶 explicit consult mode，不可由欄位有無推斷 generic/profile path
- builder 應依 `analysisModules` 與 source `moduleKey` 選擇本次實際載入的 prompts
- app-aware prompt strategy 應在 shared prompt assembly 中只覆蓋 app-specific 區段，不重做整條 consult flow
- 對於需要 Internal private code 的理論，LinkChat 應送出 raw/stable value，Internal 應依 `appId + moduleKey + theoryVersion + factKey` 做 code mapping
- code mapping table 應由 Internal data layer 持有，不應要求 external app 直接送 Internal 私有代碼
- 若 source `moduleKey` 缺值或空值，該 source 永遠可用
- `analysisModules` 應做 `trim + lowercase + deduplicate`
- `analysisModules` 與 `moduleKey` 應符合 `^[a-z0-9][a-z0-9_-]*$`
- `common` 是保留字，不可出現在 request `analysisModules`
- LinkChat 應先在本地剔除缺資料 module；Internal 不主動替它補齊
- 若 `analysisModules=[] && text!=""`，該 request 仍合法，且只跑 common sources
- 第一版 gRPC consult 只要求純文字回應，不要求檔案輸出

### Scenario group: external HTTP access
- external HTTP route 仍保留給 multipart/form-data 與附件型整合
- external app 查詢 builders 時，必須帶 `X-App-Id`
- 系統只回傳該 app 被授權且 active 的 builders
- external consult 時，系統必須驗證 `appId -> builderId`
- `POST /api/external/consult` 仍需支援附件與圖片，因此使用 `multipart/form-data`

### Scenario group: public HTTP prompt-strategy testing
- `POST /api/consult` 可接受 optional `appId`，作為 local/dev prompt-strategy testing 入口
- `POST /api/profile-consult` 可接受 optional `appId` 與 structured profile payload，作為 local/dev profile prompt-strategy testing 入口
- public HTTP 的 optional `appId` 只影響 prompt strategy selection，不承擔 external app 授權語意
- `appId` 缺值時，系統應回退到 default prompt strategy
- public HTTP prompt-strategy testing routes 僅供 local/dev 使用，不應在 production 直接對公網暴露

### Scenario group: graph
- admin 可讀取指定 builder 的 graph
- admin 可儲存 graph，且需保留 system source，對非系統 source 做 canonical reorder
- 若輸入形狀不合法或違反 graph 規則，系統應拒絕儲存
- source 應支援 optional `moduleKey`

### Scenario group: templates
- admin 可查詢指定 builder 的 templates 與全域 templates
- admin 可建立 template，且 `templateKey` 必須唯一
- admin 可更新 template，且 canonical order 必須維持一致
- admin 刪除 template 時，必須清除 source 上的 template 引用

## Acceptance Criteria Baseline

### API compatibility
- 所有 public/admin HTTP API route 盡量保持與 Java 版一致
- request/response contract、error code、HTTP status mapping 應優先保住
- external gRPC integration surface 為 Go 版新增能力，但 builder/rag/output 的行為仍應延續相同核心規則

### Response envelope
所有 HTTP handler 都應回：

```json
{
  "success": true,
  "data": {},
  "error": null
}
```

### Public/external HTTP consult request
`POST /api/consult` 與 `POST /api/external/consult` 使用 `multipart/form-data`：
- `builderId`
- `text`
- `outputFormat`
- `files`

補充：
- `POST /api/consult` 可帶 optional `appId`，用於 local/dev prompt-strategy testing
- `POST /api/profile-consult` 使用 `application/json`，可帶 optional `appId`、`analysisModules`、`subjectProfile` 與 `text`
- `POST /api/external/consult` 的 app 身分仍由 `X-App-Id` header 決定

### gRPC `ProfileConsult` request
LinkChat profile-analysis 這條線的 gRPC `ProfileConsult` request 應至少包含：
- `appId`
- `builderId`
- `analysisModules[]`
- `subjectProfile` optional
- `text` optional
- `clientIp` optional

`subjectProfile` 應至少包含：
- `subjectId`
- `modulePayloads[]`

每個 `modulePayload` 應至少包含：
- `moduleKey`
- `theoryVersion` optional, required when the module uses Internal-side code mapping
- `facts[]`

每個 `fact` 應至少包含：
- `factKey`
- `values[]`

`ProfileConsult` 補充規則：
- `analysisModules=[] && text!=""` 仍是合法 request
- `subjectProfile` 可在 text-only profile request 中省略
- builder command 必須保留 explicit `ConsultModeProfile`
- LinkChat 的 `astrology` module 屬於 codebook-enabled module，`theoryVersion` 必填
- LinkChat 的 `mbti` module 目前不走 Internal-side code mapping，`theoryVersion` 可省略

### HTTP consult response payload
`data` 維持 Java `ConsultBusinessResponse` 形狀：

```json
{
  "status": true,
  "statusAns": "",
  "response": "AI response text",
  "file": {
    "fileName": "qa-smoke-doc-consult.xlsx",
    "contentType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    "base64": "..."
  }
}
```

### gRPC `ProfileConsult` response baseline
- 第一版 profile-analysis 只要求純文字 `response`
- 若 builder `includeFile=false`，不應產出 file payload

### Prompt assembly acceptance
第一版目標與 Java 一致，並加入 app-aware structured profile/context block：
1. framework header
2. `[RAW_USER_TEXT]`
3. app-aware profile/context block when structured subject profile exists
4. selected sources by order
5. each selected source's resolved RAG by order
6. optional `[USER_INPUT]`
7. `[FRAMEWORK_TAIL]`

app-aware profile/context block 補充規則：
- default strategy 應維持既有 `[SUBJECT_PROFILE]` markdown section 風格
- LinkChat strategy 可在 shared prompt skeleton 內改用自己的 profile/context render 形式
- `modulePayloads[]` 依 `moduleKey ASC`
- `facts[]` 依 `factKey ASC`
- `values[]` 保持原始順序
- 多值以 `|` 連接，value 內的 `\` 應 escape 為 `\\`
- 多值以 `|` 連接，value 內的 `|` 應 escape 為 `\|`
- 對 codebook-enabled module，strategy 應在 render 前先依 `appId + moduleKey + theoryVersion + factKey` 將 raw value 轉成 Internal private code
- LinkChat strategy 若有輸出 `THEORY_CODEBOOK`，應維持放在 `[SUBJECT_PROFILE]` 後、第一個 source block 前，讓後續 source prompts 可直接引用該 codebook
- `THEORY_CODEBOOK` 的職責是提供模型解碼規則，不應取代 source blocks 本身的任務指令

### Module-aware source selection acceptance
- `analysisModules` 只代表本次實際參與分析的 modules
- `analysisModules` 的輸入順序不帶業務優先權
- source `moduleKey` 缺值或空值時，永遠參與 prompt assembly
- source `moduleKey` 有值時，只有當它存在於 `analysisModules` 時才參與
- `analysisModules` 必須先做 `trim + lowercase + deduplicate`
- `analysisModules` 不可包含保留字 `common`
- `ConsultModeProfile` 必須由 transport / gatekeeper 明確設定
- `analysisModules=[] && text!=""` 時，不可回退成 generic consult
- source 的最終順序仍由 `orderNo` 與既有 canonical order 決定

### Override acceptance
- 第一版以 Java 現行行為為準
- overridable RAG 是否最終套用，由 builder 模組決定

## BDD To TDD Mapping
BDD 是需求與驗收層；TDD 是實作層。

第一版開發以 UseCase 測試為主，再補 Service / Repository / Handler / gRPC transport 測試。

第一批必測：
- consult orchestration
- graph save/load
- template CRUD / reorder
- prompt assembly
- module-aware source selection
- rag resolve
- output render
- grpcapi transport mapping

測試必須回答：
- 這個 scenario 的成功條件是否成立
- 失敗條件是否被正確拒絕
- 與 Java 相容的行為是否被保住
- LinkChat structured contract 是否被穩定承接

## Public API Surface

### Public HTTP
- `GET /api/builders`
- `POST /api/consult`

### External HTTP
- `GET /api/external/builders`
- `POST /api/external/consult`

### External gRPC
- `IntegrationService/ListBuilders`
- `IntegrationService/Consult`
- `IntegrationService/ProfileConsult`

### Admin HTTP
- `GET /api/admin/builders/{builderId}/graph`
- `PUT /api/admin/builders/{builderId}/graph`
- `GET /api/admin/builders/{builderId}/templates`
- `GET /api/admin/templates`
- `POST /api/admin/templates`
- `PUT /api/admin/templates/{templateId}`
- `DELETE /api/admin/templates/{templateId}`

## Runtime Baseline
- Go 1.25
- 1 個 Go module：`com.citrus.internalaicopilot`
- `ProfileConsult` 基礎 contract 已落成 production proto 與程式碼；app-aware prompt strategy、`theoryVersion` 與 codebook extension 先以 docs-first 對齊
- app prompt config 與 theory mapping cache 目前為 process-local read-through cache，未提供 TTL 或主動 invalidation；若 Firestore 中的策略或 mapping 被修改，需重新啟動服務才保證吃到最新值

## Local Development Bootstrap Baseline
Go 版 local 開發需要提供與 Java `local profile + create-drop + initData` 等價的體驗。

目前這個需求的目標不是複製 Java 技術棧，而是保住相同的開發節奏：
- 每次 local 啟動時都能快速回到可預期的初始資料狀態
- 前端、API、測試可共用固定 seed data
- 不需要手動清髒資料後才能驗證 graph / template / consult 行為

第一版 baseline：
- local/dev 模式下，系統應支援 `reset and seed on start`
- 第一版正式做法採 Firestore emulator：啟動時清空開發用 collections/documents，再重新載入 `DefaultSeedData`
- local 預設使用 `Backend/GCP/firebase.json` 所對應的 emulator 設定
- 這個能力必須由明確的 local/dev config 控制，不能在非開發環境預設啟用

## Architecture Baseline
架構是為了穩定實作既定行為，不是用來取代行為規格。

### Module-first

```text
Go/
├── cmd/api/main.go
└── internal/
    ├── app/
    ├── gatekeeper/
    ├── grpcapi/
    ├── builder/
    ├── rag/
    ├── aiclient/
    ├── output/
    └── infra/
```

### Four-layer inside modules

```text
Handler -> UseCase -> Service -> Repository
```

`grpcapi` 是 transport adapter，主要承接 gRPC 入口，再轉交 gatekeeper usecase。

### Why this architecture is chosen
- 保留 Java 版自訂 `UseCase` 層的價值
- 讓主要 use case 能直接對應測試案例
- 將非同步 orchestration 集中在 UseCase
- 增加 AI 協作時的可預測性
- 讓 HTTP 與 gRPC 兩種 transport 都能共用相同的 domain 邊界

## Logical Modules

### app
- process wiring
- router / server setup
- grpc registration

### gatekeeper
- HTTP request parsing
- consult guard
- client IP resolve
- external app validation
- structured consult validation
- optional public `appId` pass-through for prompt-strategy testing

### grpcapi
- gRPC transport adapter
- protobuf mapping
- gRPC status mapping
- client IP fallback

### builder
- consult orchestration
- source/template domain
- graph save/load
- template CRUD
- prompt assembly
- app-aware prompt strategy dispatch
- module-aware source selection
- override

### rag
- rag config resolution
- retrieval mode dispatch
- 未來 vector/external retrieval 擴充點

### aiclient
- OpenAI Responses API
- attachment upload
- structured output parse

### output
- output policy
- markdown/xlsx render
- base64 file payload

### infra
- Firestore repository implementation
- config
- API response
- error handling
- app wiring
- seed/bootstrap
- app prompt config 與 code mapping table persistence / cache

## Java -> Go Mapping

| Java | Go | 說明 |
|------|----|------|
| gatekeeper | gatekeeper | 幾乎 1:1 |
| builder | builder | 保留 orchestrator 角色 |
| source | builder | 併入 builder domain |
| rag | rag | 保留獨立能力，並擴充 retrieval 能力 |
| aiclient | aiclient | 幾乎 1:1 |
| output | output | 幾乎 1:1 |
| common + initData | infra | Firestore / config / error / bootstrap |
| integration transport | grpcapi | Go 版新增 gRPC adapter，承接 external structured consult |

## Builder / Source / RAG Boundary

### source stays inside builder
理由：
- source 沒有獨立對外 use case
- graph / consult 都從 builder 出發
- Firestore 下適合與 builder 一起建模

### rag stays independent
理由：
- RAG 會持續成長
- builder 不應知道 retrieval 細節
- `retrievalMode` 是清楚的擴充點

設計原則：
- builder owns order
- builder owns module selection
- rag owns resolution

## Firestore Baseline Model

### `builders/{builderId}`
對應 Java `rb_builder_config`。

欄位基準：
- `builderCode`
- `groupKey`
- `groupLabel`
- `name`
- `description`
- `includeFile`
- `defaultOutputFormat`
- `filePrefix`
- `active`

### `builders/{builderId}/sources/{sourceId}`
對應 Java `rb_source`。

欄位基準：
- `prompts`
- `orderNo`
- `systemBlock`
- `moduleKey` optional
- `copiedFromTemplateId`
- `copiedFromTemplateKey`
- `copiedFromTemplateName`
- `copiedFromTemplateDescription`
- `copiedFromTemplateGroupKey`

`moduleKey` 規則：
- 缺值或空值代表此 source 永遠可用
- `common` 屬於保留語意，write path 應正規化為缺值 / 空值
- 有值時，builder 應依 request `analysisModules` 決定是否載入

### `builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}`
對應 Java source 底下的 rag config。

欄位基準：
- `ragType`
- `title`
- `content`
- `orderNo`
- `overridable`
- `retrievalMode`
- `retrievalRef` optional

### `templates/{templateId}`
對應 Java `rb_source_template` + `rb_rag_template`。

欄位基準：
- `templateKey`
- `name`
- `description`
- `groupKey`
- `orderNo`
- `prompts`
- `active`

### `templates/{templateId}/templateRags/{templateRagId}`
對應 Java template 底下的 rag config。

欄位基準：
- `ragType`
- `title`
- `content`
- `orderNo`
- `overridable`
- `retrievalMode`
- `retrievalRef` optional

### `apps/{appId}`
external app registry，用於 app-level 授權。

欄位基準：
- `appId`
- `name`
- `description`
- `active`
- `allowedBuilderIds`
- `serviceAccountEmails`

### `appPromptConfigs/{appId}`
app-aware prompt strategy registry，用於決定不同 `appId` 應走哪一套 prompt strategy。

欄位基準：
- `appId`
- `strategyKey`
- `active`

### `theoryMappings/{mappingId}`
Internal codebook mapping table，用於將 external app 傳來的 raw theory value 轉成 Internal private code。

欄位基準：
- `appId`
- `moduleKey`
- `theoryVersion`
- `slotKey`
- `rawValue`
- `internalCode`
- `interpretation`
- `active`

### Future retrieval backing stores
這些屬於 rag module 內部資料，不屬於 builder graph：
- `rag_sources/{ragSourceId}`
- `rag_vectors/{vectorId}`

目前只保留擴充空間，不列為第一版必做。

## Concurrency Baseline
Go 版將以：
- `context`
- `errgroup`
- goroutine

作為主要非同步工具。

原則：
- orchestration concurrency 放在 UseCase
- 最終 prompt 組裝必須 deterministic

## Open Questions
以下仍屬待定，不應提前寫死：
- `ProfileConsult` contract 的後續演進與相容性策略
- 是否保留 Java graph request 的 `aiagent[]` 相容輸入
- HTTP router 使用哪個套件
- XLSX 套件選擇
- dynamic/vector retrieval 的具體 storage schema
- 部署平台細節
