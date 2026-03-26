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
- LinkChat profile-analysis 這條線採單一 builder + app-aware strategy + LinkChat 第二層 analysis factory
- LinkChat profile-analysis 專用入口為 `ProfileConsult`
- LinkChat 在 hot path 不依賴先查 builders discovery，再做 consult

## Delivery Flow
本專案的預設交付順序如下：

```text
需求進入
   │
   ▼
Step 1: 定義行為
   ├─ actor
   ├─ 成功 / 失敗條件
   └─ 邊界情境
   │
   ▼
Step 2: 文件定稿
   ├─ PLAN
   ├─ DEVELOPMENT
   └─ module spec / BDD
   │
   ▼
Step 3: 測試映射
   ├─ UseCase tests
   └─ Service / Transport tests
   │
   ▼
Step 4: 最小實作
   ├─ 先讓測試紅
   └─ 再補最小 code 轉綠
   │
   ▼
Step 5: 重構
   └─ 不改既有 scenario 結果
   │
   ▼
Step 6: 文件同步
   └─ 行為變動就回寫文件
```

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

```text
consult request 進入
        │
        ▼
┌───────────────────┐
│ builderId 存在且   │──── 否 ──→ 回傳對應錯誤（not found / inactive）
│ active = true？   │
└───────┬───────────┘
        │ 是
        ▼
┌───────────────────┐
│ outputFormat 合法  │──── 否 ──→ 回傳 validation error
│ 且附件未違反限制？ │
└───────┬───────────┘
        │ 是
        ▼
  載入 builder / source
        │
        ▼
  resolve RAG
        │
        ▼
  組裝 prompt
        │
        ▼
┌───────────────────┐
│ preview mode      │──── 是 ──→ 直接回傳完整 AI request preview
│ 開啟？            │          （不呼叫 GPT）
└───────┬───────────┘
        │ 否
        ▼
  呼叫 AI
        │
        ▼
┌───────────────────┐
│ builder 需要      │──── 是 ──→ 依 output policy 回傳檔案 payload
│ 輸出檔案？        │
└───────┬───────────┘
        │ 否
        ▼
  回傳純文字結果
```

### Scenario group: LinkChat profile-analysis integration

#### 子圖 A：ProfileConsult 請求流

```text
LinkChat（external app）
        │
        │  固定 config: appId + builderId
        │  dedicated gRPC surface（不走 builders discovery 熱路徑）
        ▼
┌──────────────────────────┐
│   ProfileConsult request │
│  ┌────────────────────┐  │
│  │ appId              │  │
│  │ builderId          │  │
│  │ subjectProfile?    │  │  ← optional structured profile
│  │ text?              │  │  ← optional 補充輸入
│  └────────────────────┘  │
└────────────┬─────────────┘
             ▼
     Internal gRPC 入口
             │
             ▼
   驗證 appId → builderId
             │
             ▼
   builder command 保留 appId
   + 設定 explicit ConsultModeProfile
   （不可由欄位有無推斷 generic/profile path）
             │
             ▼
   builder 先載入整體 source / rag 骨架
             │
             ▼
   第一層 strategy
   appId=""        -> default
   appId=linkchat  -> linkchat
             │
             ▼
   LinkChat 第二層 factory
   analysisType=astrology / mbti / ...
             │
             ▼
   shared prompt skeleton
   + app-specific profile/context block
             │
             ▼
   回傳純文字回應
   （第一版不要求檔案輸出）
```

#### 子圖 B：驗證與正規化決策樹

```text
subjectProfile 輸入
        │
        ▼
subjectId 檢查
        │
        ▼
analysisPayloads[] 逐一檢查
        │
        ├── analysisType 為空？ ─────── 是 ──→ 拒絕 request
        │
        ├── analysisType 重複？ ─────── 是 ──→ 拒絕 request
        │
        ├── theoryVersion 空白？ ────── 是 ──→ 拒絕 request
        │
        ├── appId=linkchat
        │   且 analysisType=astrology
        │   但沒帶 theoryVersion？ ──── 是 ──→ 拒絕 request
        │
        └── 其餘共享 envelope 合法
                 │
                 ▼
        轉交 builder strategy
```

#### 子圖 C：Code Mapping 流

```text
LinkChat 送出 raw/stable theory value
        │
        ▼
┌───────────────────────────────────┐
│ Internal data layer 查詢          │
│ code mapping table                │
│ key = appId + analysisScopeKey    │
│       + theoryVersion + factKey   │
└───────────┬───────────────────────┘
            ▼
  raw value → Internal private code
        │
        ▼
  app-specific prompt strategy
  使用轉換後的 code 組裝
  profile/context block
```

補充規則：
- code mapping table 由 Internal data layer 持有，不要求 external app 直接送 Internal 私有代碼
- LinkChat 若需更細的欄位/value 語意組裝，應在 app-specific prompt strategy 內自行做 key resolution，不改變既有 source / RAG 大結構
- source 仍是主 prompt 骨架；RAG 仍掛在 source 底下作為補充內容；strategy 只決定要用哪些 key 去查、去組 profile/context block
- LinkChat 應先在本地剔除缺資料的 module；Internal 不主動替它補齊

### Scenario group: external HTTP access

```text
external app
        │
        ├──── GET /api/external/builders
        │         │
        │         ▼
        │   ┌─────────────────────┐
        │   │ 帶 X-App-Id header？│──── 否 ──→ 拒絕（缺少 app 身分）
        │   └────────┬────────────┘
        │            │ 是
        │            ▼
        │     以 appId 篩選
        │     授權且 active 的 builders
        │            │
        │            ▼
        │      回傳 builders 清單
        │
        └──── POST /api/external/consult
                  │  multipart/form-data（支援附件與圖片）
                  ▼
            ┌─────────────────────────┐
            │ 驗證 appId → builderId  │──── 失敗 ──→ 拒絕（未授權）
            └────────┬────────────────┘
                     │ 通過
                     ▼
              進入 consult 流程
```

補充：external HTTP route 仍保留給 multipart/form-data 與附件型整合場景。

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
- `POST /api/profile-consult` 使用 `application/json`，可帶 optional `appId`、`subjectProfile` 與 `text`
- `POST /api/external/consult` 的 app 身分仍由 `X-App-Id` header 決定

### gRPC `ProfileConsult` request
LinkChat profile-analysis 這條線的 gRPC `ProfileConsult` request 應至少包含：
- `appId`
- `builderId`
- `subjectProfile` optional
- `text` optional
- `clientIp` optional

`subjectProfile` 應至少包含：
- `subjectId`
- `analysisPayloads[]`

每個 `analysisPayload` 應至少包含：
- `analysisType`
- `theoryVersion` optional, required when the analysis type uses Internal-side code mapping
- `payload`

`ProfileConsult` 補充規則：
- `subjectProfile` 可在 text-only profile request 中省略
- builder command 必須保留 explicit `ConsultModeProfile`
- LinkChat 的 `astrology` analysis type 屬於 codebook-enabled analysis，`theoryVersion` 必填
- LinkChat 的 `mbti` analysis type 目前不走 Internal-side code mapping，`theoryVersion` 可省略

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

第一版目標與 Java 一致，並加入 app-aware structured profile/context block。

#### Prompt 組裝管線圖

```text
Step 1   ┌──────────────────────┐
         │  framework header    │
         └──────────┬───────────┘
                    ▼
Step 2   ┌──────────────────────┐
         │  [RAW_USER_TEXT]     │
         └──────────┬───────────┘
                    ▼
Step 3   ┌──────────────────────────────────────────┐
         │  app-aware profile/context block          │
         │  （僅當 structured subjectProfile 存在時） │
         │                                          │
         │  ┌─────────────────────────────────┐     │
         │  │ strategy 選擇：                  │     │
         │  │  ├── default → [SUBJECT_PROFILE] │     │
         │  │  │   markdown section 風格       │     │
         │  │  └── LinkChat → 自訂            │     │
         │  │      profile/context render      │     │
         │  └─────────────────────────────────┘     │
         └──────────┬───────────────────────────────┘
                    ▼
      ┌─────────────────────────────────────────┐
      │ THEORY_CODEBOOK（若 LinkChat 有輸出）    │
      │ 放在 [SUBJECT_PROFILE] 後、              │
      │ 第一個 source block 前                   │
      │ 職責：提供模型解碼規則                    │
      │ （不取代 source blocks 的任務指令）       │
      └──────────┬──────────────────────────────┘
                 ▼
Step 4   ┌──────────────────────────────────┐
         │  selected sources（依 orderNo）  │
         │        │                         │
Step 5   │        ▼                         │
         │  each source 的 resolved RAG     │
         │  （依 orderNo）                  │
         └──────────┬───────────────────────┘
                    ▼
Step 6   ┌──────────────────────┐
         │  [USER_INPUT]        │
         │  （optional）        │
         └──────────┬───────────┘
                    ▼
Step 7   ┌──────────────────────┐
         │  [FRAMEWORK_TAIL]    │
         └──────────────────────┘
```

#### Profile/Context Block 渲染規則

```text
analysisPayloads[] 輸入
        │ 依 analysisType ASC 排序
        ▼
  第一層 strategy
  default / linkchat
        │
        ▼
  LinkChat 第二層 factory
  依 analysisType 分流
        │
        ▼
┌───────────────────────────────────┐
│ codebook-enabled analysis？       │
└───────┬───────────────┬───────────┘
        │ 是            │ 否
        ▼               ▼
  依 appId + analysis scope key
  + theoryVersion       原始 value
  + factKey
  查 code mapping
  raw → Internal code
        │               │
        ├───────────────┘
        ▼
  facts[] 依 factKey ASC 排序
        │
        ▼
  values[] 保持原始順序
        │
        ▼
  多值以 "|" 連接
  ├── value 內 "\" → escape 為 "\\"
  └── value 內 "|" → escape 為 "\|"
        │
        ▼
  輸出 profile/context block
```

補充規則：
- `THEORY_CODEBOOK` 的職責是提供模型解碼規則，不應取代 source blocks 本身的任務指令
- 若 LinkChat 之後需要以 `factKey` / `valueKey` 做更細的語意片段組裝，該解析與拼接責任應落在 LinkChat strategy 內；既有 source/rag collections 不因這個需求改成 `factKey -> source` 或 `value -> rag` 結構

### Module-aware source selection acceptance

```text
                    request 進入
                        │
                        ▼
            ┌───────────────────────────┐
            │ ConsultModeProfile 已由    │──── 否 ──→ 不適用 module-aware
            │ transport / gatekeeper    │           selection
            │ 明確設定？                │
            └───────────┬───────────────┘
                        │ 是
                        ▼
              builder 載入整體 source
              + sourceRags 骨架
                        │
                        ▼
            ┌───────────────────────────┐
            │ appId = linkchat？         │──── 否 ──→ default strategy
            └───────────┬───────────────┘           保留 generic/common tags
                        │ 是
                        ▼
              逐一讀取 analysisPayloads[]
                        │
                        ▼
              第二層 factory 依 analysisType
              產出 internal selection keys
                        │
                        ▼
              遍歷所有 source
                        │
          ┌─────────────┴─────────────┐
          ▼                           ▼
  source 無 internal tag        source 有 internal tag
          │                           │
          ▼                           ▼
  永遠參與 shared skeleton    交給 LinkChat strategy
                              判斷是否參與
                              （可看 source.moduleKey
                               或其他 internal key）
                                    │
                                    ▼
                      最終順序依 orderNo 與
                      既有 canonical order 決定
```

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
- `ProfileConsult`、app-aware prompt strategy、`theoryVersion` 與 codebook extension 已落成 proto 與 production code
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
Handler → UseCase → Service → Repository
  │          │         │          │
  │  HTTP/   │ 業務    │ domain   │ 資料
  │  gRPC    │ 編排    │ 邏輯     │ 存取
  │  入口    │ + 並行  │          │ (Firestore)
```

`grpcapi` 是 transport adapter，主要承接 gRPC 入口，再轉交 gatekeeper usecase。

### Why this architecture is chosen
- 保留 Java 版自訂 `UseCase` 層的價值
- 讓主要 use case 能直接對應測試案例
- 將非同步 orchestration 集中在 UseCase
- 增加 AI 協作時的可預測性
- 讓 HTTP 與 gRPC 兩種 transport 都能共用相同的 domain 邊界

## Logical Modules

### 模組互動 / 資料流圖

```text
                    ┌─────────────────────────────────────────────────┐
                    │                   Transport 層                   │
                    │                                                 │
  HTTP request ───→ │  gatekeeper (HTTP handler)                      │
                    │    │  request parsing / consult guard            │
                    │    │  external app validation                    │
                    │    │  structured consult validation              │
                    │    │  client IP resolve                          │
                    │    │  optional appId pass-through                │
                    │                                                 │
  gRPC request ───→ │  grpcapi (gRPC transport adapter)               │
                    │    │  protobuf mapping                          │
                    │    │  gRPC status mapping                       │
                    │    │  client IP fallback                        │
                    └────┼────────────────────────────────────────────┘
                         │
                         ▼
                    ┌─────────────────────────────────────────────────┐
                    │              Domain / Orchestration 層           │
                    │                                                 │
                    │  builder                                        │
                    │    ├── consult orchestration（主編排）           │
                    │    ├── source / template domain                 │
                    │    ├── graph save / load                        │
                    │    ├── template CRUD                            │
                    │    ├── prompt assembly                          │
                    │    ├── app-aware prompt strategy dispatch       │
                    │    ├── module-aware source selection            │
                    │    └── override                                 │
                    │         │                    │                  │
                    │         ▼                    ▼                  │
                    │  rag                   aiclient                 │
                    │    ├── config           ├── OpenAI Responses    │
                    │    │   resolution       │   API                 │
                    │    ├── retrieval        ├── attachment upload   │
                    │    │   mode dispatch    └── structured output   │
                    │    └── 未來 vector/          parse              │
                    │       external 擴充                             │
                    └────────────────────────┬────────────────────────┘
                                            │
                                            ▼
                    ┌─────────────────────────────────────────────────┐
                    │                   Output 層                     │
                    │                                                 │
                    │  output                                         │
                    │    ├── output policy                            │
                    │    ├── markdown / xlsx render                   │
                    │    └── base64 file payload                      │
                    └────────────────────────┬────────────────────────┘
                                            │
                                            ▼
                    ┌─────────────────────────────────────────────────┐
                    │                Infrastructure 層                │
                    │                                                 │
                    │  app                          infra             │
                    │    ├── process wiring           ├── Firestore   │
                    │    ├── router / server          │   repository  │
                    │    │   setup                    ├── config      │
                    │    └── grpc registration        ├── API response│
                    │                                 ├── error       │
                    │                                 │   handling    │
                    │                                 ├── app wiring  │
                    │                                 ├── seed /      │
                    │                                 │   bootstrap   │
                    │                                 └── app prompt  │
                    │                                     config &   │
                    │                                     code mapping│
                    │                                     cache       │
                    └─────────────────────────────────────────────────┘
```

### 模組間呼叫方向

```text
gatekeeper ──→ builder ──→ rag
    │              │
    │              ├──→ aiclient
    │              │
    │              └──→ output
    │
grpcapi ───→ gatekeeper
    │
app ───→ gatekeeper, grpcapi, infra（wiring）
    │
所有模組 ──→ infra（Firestore repository, config, error handling）
```

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

```text
┌─────────────────────────────────────────────────────────────┐
│                      builder module                         │
│                                                             │
│  builder owns:                                              │
│    ├── order（source 排序）                                  │
│    └── source participation                                │
│       （依 app strategy / analysis factory 決定）           │
│                                                             │
│  ┌───────────────────────────────────┐                      │
│  │           source                  │                      │
│  │  （併入 builder domain）          │                      │
│  │                                   │                      │
│  │  理由：                           │                      │
│  │   • 無獨立對外 use case           │                      │
│  │   • graph / consult 皆從          │                      │
│  │     builder 出發                  │                      │
│  │   • Firestore 下適合與            │                      │
│  │     builder 一起建模              │                      │
│  └───────────────┬───────────────────┘                      │
│                  │ 引用                                      │
└──────────────────┼──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│                      rag module（獨立）                      │
│                                                             │
│  rag owns:                                                  │
│    └── resolution（RAG config 解析與內容檢索）               │
│                                                             │
│  理由：                                                     │
│   • RAG 會持續成長                                          │
│   • builder 不應知道 retrieval 細節                         │
│   • retrievalMode 是清楚的擴充點                            │
└─────────────────────────────────────────────────────────────┘
```

```text
呼叫方向：

builder ──owns──→ source ordering
builder ──owns──→ module selection
builder ──calls─→ rag.resolve(sourceRagConfig)
rag ─────owns──→ resolution / retrieval logic
```

## Firestore Baseline Model

### Collection 關聯圖（ER Diagram）

```text
builders/{builderId}
│  builderCode, groupKey, groupLabel, name,
│  description, includeFile, defaultOutputFormat,
│  filePrefix, active
│
├──► sources/{sourceId}                          （子集合）
│    │  prompts, orderNo, systemBlock,
│    │  moduleKey?, copiedFromTemplate*
│    │
│    └──► sourceRags/{ragId}                     （子集合）
│         ragType, title, content, orderNo,
│         overridable, retrievalMode, retrievalRef?
│
│
│  ┌─ allowedBuilderIds ─────────────────────────（引用）
│  │
apps/{appId}
│  appId, name, description, active,
│  allowedBuilderIds[], serviceAccountEmails[]
│
│  ┌─ appId ─────────────────────────────────────（引用）
│  │
appPromptConfigs/{appId}
│  appId, strategyKey, active
│
│
│  ┌─ appId + analysisScopeKey ──────────────────（引用）
│  │
theoryMappings/{mappingId}
   appId, analysisScopeKey, theoryVersion, slotKey,
   rawValue, internalCode, interpretation, active


templates/{templateId}                            （獨立集合）
│  templateKey, name, description, groupKey,
│  orderNo, prompts, active
│
└──► templateRags/{templateRagId}                 （子集合）
     ragType, title, content, orderNo,
     overridable, retrievalMode, retrievalRef?


── 未來擴充（不列為第一版必做）──

rag_sources/{ragSourceId}                         （rag module 內部）
rag_vectors/{vectorId}                            （rag module 內部）
```

### 關聯摘要

```text
apps.allowedBuilderIds[] ───references──→ builders/{builderId}
appPromptConfigs.appId   ───references──→ apps/{appId}
theoryMappings.appId     ───references──→ apps/{appId}
theoryMappings.moduleKey ───corresponds─→ sources.moduleKey
source.copiedFromTemplate*───copied-from─→ templates/{templateId}
```

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
- 有值時，作為 strategy 可選用的 internal tag，不再要求由 top-level module list 直接驅動

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
- `analysisScopeKey`
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
