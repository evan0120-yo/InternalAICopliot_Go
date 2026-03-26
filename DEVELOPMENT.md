# Internal AI Copilot Go Backend Development Guide

## AI Delivery Rule
以下規則在 AI 參與 Go 專案開發時必須優先遵守：

1. AI 開發不應自行套用人類常見的「權重排序」思維來把需求拆成只做一部分。所謂優先級通常是給人類協作與排程使用；當需求已經交給 AI 執行時，除非有明確限制、風險或特殊理由，否則操作者預設期待 AI 一次花時間把整件事完整處理到可交付。
2. 操作者會希望一次花時間搞定，因為就可以不被 AI 打斷去做其他事情。
3. 反之就要一直顧 AI 會非常累。
4. 因此，除非需求方明確要求分段、保留後續任務、只做部分、先做 spike，或 AI 遇到必須先停下來確認的 blocker，否則預設應以一次完整交付為原則。

## Collaboration Guard Rule
以下規則在 Go 專案協作時必須優先遵守：

1. 嚴禁 AI 替需求方決定事情；遇到方向、範圍、取捨、過渡方案與最終方案選擇時，必須先問需求方，再進入實作。
2. AI 可以主動提出骨架版、閹割版、過渡版、簡化版或其他替代方案作為選項，但不能跳過需求方自行決定採用哪一種方案，更不能未經確認就直接實作；除非情況特殊，否則預設應以完整版為主。

## BDD-First Strict Rule
本專案的 Go 開發流程採 `BDD-first`，且需嚴格執行。

嚴格執行的意思是：
- 先和需求方確認業務行為、情境、例外與驗收條件，再進入實作討論
- 先產出並維護對應行為規格文件，之後的測試與程式碼必須對齊該規格
- 若文件、測試、實作不一致，必須優先修正不一致本身，而不是憑記憶繼續開發
- AI 協作時，應主動補齊行為導向問題，不得直接以實作假設取代需求確認

## Purpose
這份文件定義 Go 版 Backend 的開發規則。目標不是追求某種架構流派的純度，而是讓以下幾件事同時成立：

1. Go 版對外 public/admin HTTP 行為盡量和 Java 版一致
2. Firestore / RAG 演進空間要先保留
3. LinkChat 這類 external profile-analysis app 可以用固定 builder 的 structured gRPC contract 穩定接入
4. 人與 AI 協作開發時，需求、文件、測試、實作要能穩定同步

這份文件是 Go 版開發時的主要規範。若文件與程式碼不一致，應優先修文件或修程式碼，不要靠口頭記憶補齊。

## Primary Development Flow

```text
┌─────────────────────────────────────────────┐
│  Step 1: Define Behavior First              │
│  先以使用情境、行為與驗收條件定義需求        │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│  Step 2: Document the Agreed Behavior       │
│  寫入 PLAN / 開發文件 / module 文件         │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│  Step 3: Map Behavior to Tests              │
│  BDD 定義行為驗收 → TDD 最小步驟落實       │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│  Step 4: Implement the Minimum Code         │
│  先寫失敗測試 → 再補最小實作讓其通過       │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│  Step 5: Refactor Without Changing Behavior │
│  只改善結構，不改變既有 scenario 結果       │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│  Step 6: Sync Docs After Change             │
│  行為若變動 → 同步更新文件                  │
└─────────────────────────────────────────────┘
```

**Step 1 至少要先確認：**
- actor 是誰
- 目標行為是什麼
- 成功條件是什麼
- 失敗條件是什麼
- 邊界情境是什麼
- 哪些地方要維持 Java 相容
- external integration 是否走 HTTP 或 gRPC

**Step 2 文件至少要描述：**
- scenario summary
- acceptance criteria
- open questions
- 對應 module boundary
- request/response contract

**Step 3 補充：**
- 測試的來源應是已確認的 scenario，而不是實作者臨時推測的流程。

## Local Reset And Seed Rule
Go 版 local 開發必須支援一種與 Java `create-drop + initData` 等價的 bootstrap 模式。

目標：
- 每次 local 啟動時都能回到固定初始資料
- 讓前端、API 與測試可直接依賴同一批 seed data
- 降低 graph/template/consult 開發時的髒資料干擾

目前正式做法是 Firestore emulator：
- local app 與測試預設走 `FIRESTORE_EMULATOR_HOST=localhost:8090`
- project id 預設為 `dailo-467502`
- reset and seed on start 的做法是清空開發用 collections/documents 後重新載入 `DefaultSeedData`

執行規則：
- 只能在明確 local/dev 模式或顯式開關下啟用
- 不能在未受保護的正式環境啟用 reset
- seed data 應集中在 infra/bootstrap 定義，避免散落在 entrypoint 與各 module
- seed 的目的應是提供穩定開發基線，不應偷偷承載 production migration 責任

目前主要相關環境變數：
- `INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID`
- `INTERNAL_AI_COPILOT_FIRESTORE_EMULATOR_HOST`
- `INTERNAL_AI_COPILOT_STORE_RESET_ON_START`

舊前綴相容：
- `REWARDBRIDGE_*` 目前仍保留 fallback，相容既有本機與部署設定

## External App Access Rule
對外整合系統目前採雙 gatekeeper 模型：

```text
External App Request
       │
       ↓
┌──────────────────────────────────────────┐
│  Gate 1: External App 本地 Gatekeeper    │
│  ├── 業務授權檢查                        │
│  └── 資料完整性處理                      │
└──────────────────┬───────────────────────┘
                   │ 通過
                   ↓
┌──────────────────────────────────────────┐
│  Gate 2: Internal Gatekeeper             │
│  ├── 服務邊界驗證                        │
│  └── Builder 授權驗證                    │
└──────────────────┬───────────────────────┘
                   │ 通過
                   ↓
          進入業務流程處理
```

### LinkChat profile-analysis baseline
LinkChat 這條線目前的 agreed behavior：

```text
LinkChat 發起 Profile Analysis 請求
       │
       ↓
┌─────────────────────────────────────────────────────┐
│  協定：LinkChat ←── gRPC ──→ Internal               │
│  專用入口：ProfileConsult                            │
│  Hot path 不應每次先查 ListBuilders                  │
│  LinkChat 以 config 固定 appId + builderId           │
└──────────────────────┬──────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────┐
│  組裝 Request                                        │
│  ├── subjectProfile （optional structured）          │
│  └── text           （optional 補充輸入）            │
└──────────────────────┬──────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────┐
│  LinkChat 本地前處理                                 │
│  ├── 只保留有資料的 analysis payload                 │
│  ├── analysisType / theoryVersion 整理               │
│  ├── duplicate analysisType 本地拒絕                 │
│  └── 不把 LinkChat 私有資料模型直接外送              │
└──────────────────────┬──────────────────────────────┘
                       ↓
              ┌── subjectProfile 與 text 狀態？──┐
              │                                  │
  subjectProfile=nil && text=""     其餘情況
              │                                  │
              ↓                                  ↓
     LinkChat 本地短路                    呼叫 ProfileConsult
     不呼叫 Internal                      （固定 appId + builderId）
                                                  │
                                                  ▼
     第一版回應：純文字 response（不要求檔案輸出）
```

### Internal gatekeeper must validate

```text
Request 進入 Internal Gatekeeper
       │
       ↓
  ┌─ Gate 1 ─── appId 是否存在？ ──────────────── 否 → 拒絕
  │    ↓ 是
  ├─ Gate 2 ─── app 是否 active？ ─────────────── 否 → 拒絕
  │    ↓ 是
  ├─ Gate 3 ─── appId 是否允許使用指定 builderId？ 否 → 拒絕
  │    ↓ 是
  ├─ Gate 4 ─── builderId 是否存在且 active？ ──── 否 → 拒絕
  │    ↓ 是
  ├─ Gate 5 ─── subjectProfile 缺值？ ──────────── 是 → 合法 text-only profile
  │    ↓ 否
  ├─ Gate 6 ─── subjectId 是否存在？ ──────────── 否 → 拒絕
  │    ↓ 是
  ├─ Gate 7 ─── analysisPayloads 有 duplicate      是 → 拒絕
  │             analysisType？
  │    ↓ 否
  ├─ Gate 8 ─── 每個 analysisType 合法？ ──────── 否 → 拒絕
  │    ↓ 是
  ├─ Gate 9 ─── theoryVersion 若提供是否非空白？ 否 → 拒絕
  │    ↓ 是
  └─ Gate 10 ── linkchat + astrology 缺少          是 → 拒絕
                theoryVersion？
        ↓ 否
  全部通過 → 進入業務流程
```

### LinkChat local gatekeeper owns
- 對象資料查詢
- 模組開通判斷
- 缺資料 module 剔除
- duplicate module / fact 本地拒絕
- structured payload normalization
- 空 request 本地攔截

### External HTTP route baseline
external HTTP routes 仍保留給 multipart/form-data 與附件型整合：
- `GET /api/external/builders`
- `POST /api/external/consult`

external HTTP 規則：
- external app 必須透過 header `X-App-Id` 傳送 `appId`
- `POST /api/external/consult` 仍維持 `multipart/form-data`
- 第一版 `appId -> builderId` 為主要授權規則

刻意延後的能力：
- `allowedEndpoints`
- `rateLimit`
- `serviceAccountEmail` enforcement

原因：
- 第一版目標是先收斂 `appId -> builderId` 與 structured profile consult contract
- Cloud Run service account 對應會在部署方案落定後再收斂成正式 enforcement 規則

目前測試基準：
- compile/check 可直接跑 `go test -run TestDoesNotExist -p 1 ./...`
- 完整測試應透過 `Backend/GCP` 下的 `firebase emulators:exec --project dailo-467502 --only firestore 'pwsh -Command "Set-Location ..\\Go; go test -p 1 ./..."'`

## What AI Must Do During Collaboration
當需求尚未清楚時，AI 應主動問行為導向問題，而不是直接腦補實作。

優先提問方向：
- 這個功能的 actor 與目標是什麼
- 成功時要看到什麼結果
- 失敗時要怎麼回應
- 哪些 case 屬於驗收範圍
- 是否有 Java 相容行為必須保留
- 是 HTTP contract 還是 gRPC contract
- `analysisPayloads`、`subjectProfile`、`analysisType` / `moduleKey` 等欄位語意是否已鎖定

不應優先做的事：
- 未確認 scenario 就先決定資料模型
- 未確認 acceptance criteria 就先下 production code
- 未確認 LinkChat / Internal 責任切分就直接做單邊驗證
- 只談架構分層，不談行為結果

## Primary Design Choice
Go 版採用：

- BDD-first development
- module-first
- four-layer architecture
- TDD-first development
- light DDD mindset

### BDD-first
行為規格先於技術實作。文件應優先回答「要發生什麼」，而不是先回答「怎麼實作」。

### TDD-first
在行為規格已確認後，先寫測試案例，再寫實作。UseCase 是最主要的行為測試對應層。

### Module-first
先用業務模組切開，再在模組內使用固定分層。

### Four-layer architecture

```text
Handler -> UseCase -> Service -> Repository
```

### Light DDD
保留清楚的業務邊界與責任分工，但不引入過重 ceremony。

## Why Go Still Uses Four Layers
Go 並不是不能用三層；但在本專案，保留四層有明確價值。

### Reason 1: preserve the Java architecture intent
Java 版不是單純三層，而是明確多了一層 `UseCase` 作為 orchestration layer。

這層目前負責：
- 跨模組流程編排
- 併發/非同步 orchestration
- 對應主要 use case 測試案例

### Reason 2: better BDD to TDD mapping
UseCase 最適合把已確認的 scenario 落成可執行測試：
- `Consult`
- `SaveGraph`
- `LoadGraph`
- `CreateTemplate`
- `DeleteTemplate`
- `IntegrationConsult`

若砍掉 UseCase，測試容易散落在 handler 或 service，讓案例邊界變模糊。

### Reason 3: keep orchestration and concurrency in one place
Go 版會用：
- `context.Context`
- `errgroup`
- goroutine

這些 orchestration 行為應盡量集中在 UseCase 層，而不是散落在 handler/service/repository。

### Reason 4: improve AI collaboration stability
對 AI 來說，四層架構在這個案子反而更穩定，因為：
- handler / grpc adapter 不會偷偷長業務邏輯
- usecase 不會被 service 取代成大泥球
- service 不會亂做跨模組總編排
- repository 不會亂長業務規則

結論：
- Go 版保留四層
- 但不做過重的全套 DDD ceremony

## Module Structure

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

## Layer Rules
分層是為了穩定落實行為規格，不是為了讓各層自行產生規則。

### 1. Handler / Transport Adapter

```text
╔══════════════════════════════════════════════════════╗
║  Layer 1: Handler / Transport Adapter                ║
╠══════════════════════════════════════════════════════╣
║                                                      ║
║  用途：                                              ║
║  ├── HTTP request parse                              ║
║  ├── route binding                                   ║
║  ├── multipart parse                                 ║
║  ├── gRPC request mapping                            ║
║  ├── response encode                                 ║
║  └── HTTP / gRPC status mapping                      ║
║                                                      ║
║  ✔ 可以做：                                          ║
║  ├── 讀 path/query/header/form/body                  ║
║  ├── 讀 protobuf request                             ║
║  ├── 呼叫單一 UseCase                                ║
║  └── 把 result 包成 ApiResponse 或 gRPC response     ║
║                                                      ║
║  ✘ 不能做：                                          ║
║  ├── 不直接呼叫 Repository                           ║
║  ├── 不做跨模組流程編排                              ║
║  ├── 不做 prompt assembly                            ║
║  ├── 不做 module entitlement 推論                    ║
║  └── 不做 Firestore 操作                             ║
║                                                      ║
║          │ 只能往下呼叫                              ║
║          ↓                                           ║
║      UseCase                                         ║
╚══════════════════════════════════════════════════════╝
```

### 2. UseCase

```text
╔══════════════════════════════════════════════════════╗
║  Layer 2: UseCase                                    ║
╠══════════════════════════════════════════════════════╣
║                                                      ║
║  用途：                                              ║
║  ├── 單一業務案例的流程編排中心                      ║
║  ├── 跨模組協作                                      ║
║  ├── 併發 orchestration                              ║
║  └── 對應主要 BDD scenario 的可執行測試案例          ║
║                                                      ║
║  ✔ 可以做：                                          ║
║  ├── 呼叫本模組 service                              ║
║  ├── 呼叫其他模組公開 usecase/service 介面           ║
║  ├── 建立 context / timeout / errgroup               ║
║  ├── 控制流程順序                                    ║
║  └── 收集並組裝多個 service 結果                     ║
║                                                      ║
║  ✘ 不能做：                                          ║
║  ├── 不直接寫資料存取細節                            ║
║  ├── 不把純業務邏輯都塞在 usecase 裡                 ║
║  ├── 不做和 HTTP/gRPC transport 強耦合的 parse       ║
║  └── 不自己實作 Firestore query                      ║
║                                                      ║
║          │ 往下呼叫                                  ║
║          ↓                                           ║
║      Service（+ 可跨模組呼叫其他 UseCase/Service）   ║
╚══════════════════════════════════════════════════════╝
```

### 3. Service

```text
╔══════════════════════════════════════════════════════╗
║  Layer 3: Service                                    ║
╠══════════════════════════════════════════════════════╣
║                                                      ║
║  用途：                                              ║
║  ├── 模組內純業務邏輯                                ║
║  ├── 可重用規則                                      ║
║  └── deterministic business logic                    ║
║                                                      ║
║  ✔ 可以做：                                          ║
║  ├── normalize / order / merge / validate            ║
║  ├── prompt assembly                                 ║
║  ├── output policy 判斷                              ║
║  ├── retrieval mode resolve                          ║
║  ├── override strategy                               ║
║  └── strategy / internal tag 的 prompt 組裝規則      ║
║                                                      ║
║  ✘ 不能做：                                          ║
║  ├── 不直接處理 HTTP request/response                ║
║  ├── 不直接處理 gRPC request/response                ║
║  ├── 不做跨模組總編排                                ║
║  ├── 不依賴 transport 細節                           ║
║  └── 不自己控制整體 use case 的 goroutine fan-out    ║
║                                                      ║
║          │ 往下呼叫                                  ║
║          ↓                                           ║
║      Repository                                      ║
╚══════════════════════════════════════════════════════╝
```

### 4. Repository

```text
╔══════════════════════════════════════════════════════╗
║  Layer 4: Repository                                 ║
╠══════════════════════════════════════════════════════╣
║                                                      ║
║  用途：                                              ║
║  ├── 資料存取抽象                                    ║
║  └── 對 domain 暴露存取能力                          ║
║                                                      ║
║  ✔ 可以做：                                          ║
║  ├── Firestore read/write/query/batch/transaction    ║
║  └── persistence mapping                             ║
║                                                      ║
║  ✘ 不能做：                                          ║
║  ├── 不做 prompt assembly                            ║
║  ├── 不做 output policy                              ║
║  ├── 不做 retrieval strategy                         ║
║  └── 不做 HTTP / gRPC aware 行為                     ║
║                                                      ║
║          │                                           ║
║          ↓                                           ║
║      Firestore / 外部儲存                            ║
╚══════════════════════════════════════════════════════╝
```

## Allowed Dependency Direction

```text
主要依賴方向（由上往下）：

  Handler / gRPC Adapter
          │
          ↓  ✔ handler → usecase
          │  ✔ grpcapi → gatekeeper usecase
      UseCase ─────────────────────────────┐
          │                                │
          ↓  ✔ usecase → service           │ ✔ usecase → other module
          │                                │   public boundary
      Service                              │
          │                                │ △ usecase → repository
          ↓  ✔ service → repository        │   （僅限明確 orchestration
          │                                │    理由 + 檔案註解說明）
      Repository                           │
                                           │
  ─────────────────────────────────────────┘

禁止的反向依賴（由下往上 / 跨層）：

  Handler / gRPC Adapter
          ↑  ✘ service → handler
          ↑  ✘ service → grpcapi
      UseCase
          ↑  ✘ repository → usecase
          ↑  ✘ repository → service
      Service
          ↑
      Repository
          ↑  ✘ handler → repository（跳層）
             ✘ grpcapi → repository（跳層）
             ✘ handler → service for cross-module orchestration
```

### Cross-module rule
預設只有 UseCase 層可以做跨模組協作。

```text
  Module A                    Module B
┌──────────────┐          ┌──────────────┐
│  UseCase ────┼── ✔ ───→ │  UseCase     │
│  Service     │          │  Service     │
│  Repository  │          │  Repository  │
└──────────────┘          └──────────────┘
```

例外情況必須非常少，而且要有明確理由。若沒有足夠理由，回到 module boundary in UseCase。

## Module-by-Module Rules

### app
責任：
- process wiring
- HTTP router setup
- gRPC server registration
- config/bootstrap 啟動整合

規則：
- app 不承擔業務判斷
- app 應註冊 gatekeeper HTTP handlers 與 grpcapi transport

### gatekeeper
對應 Java：
- controller
- gatekeeper usecase
- guard service
- dto

分層建議：
- `handler.go`
- `consult_usecase.go`
- `guard_service.go`
- `model.go`

責任：
- `GET /api/builders`
- `POST /api/consult`
- `GET /api/external/builders`
- `POST /api/external/consult`
- 接收 grpcapi 轉進來的 generic `Consult` command
- 接收 grpcapi 轉進來的 `ProfileConsult` command
- client IP resolve
- request validation
- external app validation
- 呼叫 builder usecase

規則：
- guard 規則放在 Service
- consult 轉交與流程控制放在 UseCase
- handler 不直接做 builder 邏輯
- gatekeeper 負責 Internal 服務邊界驗證
- gatekeeper 不負責 LinkChat 的模組開通判斷與缺資料剔除
- gatekeeper 必須把 generic consult 與 profile consult 映射成明確 `ConsultMode`

### grpcapi
這是 Go 版新增的 transport adapter，不是新的 domain module。

分層建議：
- `service.go`
- `mapper.go`
- `pb/`

責任：
- 實作 gRPC `IntegrationService`
- 將 protobuf request 轉成 gatekeeper command
- 依 RPC path 設定明確 `ConsultMode`
- 將 business error 映射為 gRPC status
- 提供 client IP fallback

規則：
- grpcapi 不做 prompt selection
- grpcapi 不做 LinkChat 本地 gatekeeping
- grpcapi 不直接調用 builder repository
- LinkChat profile-analysis hot path 不應依賴每次先呼叫 `ListBuilders`
- grpcapi 不得靠 `subjectProfile` 是否為空推斷 consult mode

### builder
對應 Java：
- builder
- source
- template 相關邏輯

分層建議：
- `consult_usecase.go`
- `graph_usecase.go`
- `template_usecase.go`
- `assemble_service.go`
- `override_service.go`
- `graph_service.go`
- `template_service.go`
- `model.go`
- `repository.go`

責任：
- consult orchestration
- source/template domain
- graph save/load
- template CRUD
- prompt assembly
- override
- module-aware source selection

規則：
- source 併入 builder domain，不獨立成 module
- usecase 負責 orchestration
- service 負責 prompt assembly / normalize / graph rules / template rules
- repository 只負責 builder/source/template persistence
- profile-analysis 第一版採單一 builder
- builder consult command 必須帶明確 `ConsultMode`
- builder 依 `builderId` 載入整體 source/rag 骨架
- source `moduleKey` 缺值或空值時永遠可用
- source `moduleKey` 有值時，作為 strategy 可選用的 internal tag
- `analysisPayloads` 的輸入順序不直接決定 prompt block 排序語意
- `subjectProfile` 由 external app 傳入，builder 只負責把它轉成 deterministic prompt block
- `subjectProfile=nil && text!=""` 在 profile mode 中仍是合法 request

### rag
對應 Java：
- rag module，但範圍更大

分層建議：
- `resolve_usecase.go` 或 `resolver.go`
- `resolve_service.go`
- `retriever_*.go`
- `model.go`
- `repository.go`

責任：
- 將 rag configs resolve 成可用內容
- 依 `retrievalMode` 分派 retrieval 策略
- 保留未來 dynamic/vector retrieval 擴充空間

規則：
- builder 不區分靜態/動態 RAG
- builder 只傳 rag config，rag 回 resolved content
- 哪些 source 會進入 rag，由 builder 先依 builder skeleton 與 strategy source filtering 決定
- override 最終是否套用由 builder 決定

### aiclient
對應 Java：
- aiclient usecase + service

分層建議：
- `analyze_usecase.go`
- `analyze_service.go`
- `model.go`

責任：
- OpenAI Responses API
- attachment upload
- structured output parse

規則：
- usecase 作為對外入口
- service 放 API request/response 細節與錯誤 mapping
- 不做 prompt assembly
- 不理解 `analysisPayloads` 與 `subjectProfile` 的業務語意

### output
對應 Java：
- output usecase + service + renderers

分層建議：
- `render_usecase.go`
- `render_service.go`
- `renderer_markdown.go`
- `renderer_xlsx.go`
- `model.go`

責任：
- output policy
- markdown/xlsx render
- file payload encode

規則：
- usecase 是對外入口
- service 決定 output policy
- renderer 只做單一格式的實作
- LinkChat profile-analysis 第一版預期為 text-only，對應 builder 應走 `includeFile=false`

### infra
責任：
- Firestore repository implementations
- config
- `ApiResponse`
- business errors
- app wiring
- dev seed/bootstrap

規則：
- infra 不承擔 domain 規則
- infra 是各 module 的依賴提供者，不是業務中心
- infra 應承接 source document 的 optional `moduleKey`
- infra 應承接 app registry 的 `allowedBuilderIds`

## Naming Rules

### UseCase names
UseCase 應對應真實業務案例，不要用含糊名稱。

推薦：
- `ConsultUseCase`
- `LoadGraphUseCase`
- `SaveGraphUseCase`
- `CreateTemplateUseCase`
- `DeleteTemplateUseCase`
- `AnalyzeUseCase`
- `RenderUseCase`

避免：
- `MainUseCase`
- `DefaultUseCase`
- `CoreUseCase`

### Service names
Service 名稱應反映純邏輯責任。

推薦：
- `GuardService`
- `AssembleService`
- `GraphService`
- `TemplateService`
- `ResolveService`
- `RenderService`

### Repository names
Repository 應反映 aggregate 或資料責任。

推薦：
- `BuilderRepository`
- `TemplateRepository`
- `RagSourceRepository`
- `AppRepository`

## Testing Strategy
本專案採 `BDD-first + TDD-first`。

### Primary rule
先定義 scenario 與 acceptance criteria，再寫 UseCase 測試，之後再補 Service、Repository、Handler、gRPC transport 測試。

原因：
- UseCase 最能鎖住 Java 相容行為
- Service 最能鎖住業務規則
- grpcapi transport 最能鎖住 external structured contract
- Repository 最能鎖住 Firestore 存取正確性

### Test priority

```text
  測試金字塔（由上往下：優先級高 → 低，覆蓋範圍窄 → 寬）

          ╱╲
         ╱  ╲
        ╱ 1  ╲         ★ 最高優先
       ╱      ╲        UseCase Tests
      ╱ Use    ╲       行為流程 + 契約驗證
     ╱  Case    ╲
    ╱────────────╲
   ╱      2       ╲    ★★ 高優先
  ╱    Service     ╲   純邏輯 + edge case
 ╱     Tests        ╲
╱────────────────────╲
╲        3            ╱ ★★★ 中優先
 ╲  Repository       ╱  persistence 正確性
  ╲   Tests         ╱
   ╲───────────────╱
    ╲      4      ╱     ★★★★ 基礎覆蓋
     ╲ Handler / ╱      transport parse + mapping
      ╲ grpcapi ╱
       ╲ Tests ╱
        ╲────╱
```

#### 1. UseCase tests
這是最重要的一層。

應優先覆蓋：
- consult orchestration
- graph save/load
- template CRUD
- module-aware source selection
- rag resolve flow
- output render flow

UseCase 測試主要回答：
- 這個 scenario 的流程順序對不對
- 哪些 dependency 被呼叫
- 錯誤如何傳遞
- 最終結果是否符合契約

#### 2. Service tests
應覆蓋：
- prompt assembly
- guard validation
- override strategy
- graph normalize rules
- template reorder rules
- output policy
- strategy source filtering / internal tag 規則
- retrieval mode resolution

Service 測試主要回答：
- 純邏輯對不對
- edge cases 對不對
- 順序與 canonicalization 對不對

#### 3. Repository tests
應覆蓋：
- Firestore document mapping
- batch write / transaction behavior
- query ordering
- update/delete semantics
- source `moduleKey` mapping
- app registry mapping

Repository 測試主要回答：
- persistence 行為對不對
- Firestore schema 假設有沒有被破壞

#### 4. Handler / grpcapi tests
transport 測試要有，但不是第一優先。

應覆蓋：
- request parse
- multipart parse
- protobuf mapping
- status code / gRPC status
- response envelope
- error mapping

### What not to do in tests
- 不要跳過 scenario 定義直接寫測試
- 不要只測 repository 而不測 usecase
- 不要把所有邏輯塞進 integration test
- 不要先寫 implementation 再補 usecase test

## Concurrency Rules
Go 版的併發原則如下：

### Primary rule
UseCase 擁有 orchestration concurrency。

意思是：
- `context`
- `errgroup`
- goroutine fan-out / wait-all

應優先放在 UseCase。

### Service concurrency
Service 預設不主動開多個 goroutine 做跨依賴協調。

若 Service 真的需要併發：
- 必須是模組內部局部演算法需求
- 不得模糊 UseCase 與 Service 的責任
- 需在檔案註解中說明原因

### Repository concurrency
Repository 不負責業務併發策略。

## Firestore Rules
第一版以以下資料模型為基準：
- `builders/{builderId}`
- `builders/{builderId}/sources/{sourceId}`
- `builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}`
- `templates/{templateId}`
- `templates/{templateId}/templateRags/{templateRagId}`
- `apps/{appId}`

規則：
- persistence 設計可以調整
- 但對外 API 與 Java 業務語意要先保住
- 若 Firestore 與 Java relational 模型衝突，先保業務契約，再調整 storage strategy
- source document 應支援 optional `moduleKey`
- `moduleKey` 缺值或空值視為 always-on source
- `common` 屬於保留語意，write path 應正規化為缺值 / 空值

## RAG Rules

### Unified RAG concept
不要把 RAG 分成兩套模型。

第一版原則：
- graph 裡存的是 `ragConfig`
- rag module 根據 `retrievalMode` resolve
- builder 不區分靜態/動態 RAG

### Required mode
- `full_context`

### Future modes
- `vector_search`
- `external_api`

這些未來模式在文件中可保留為擴充點，但不可假裝已完成。

### Boundary reminder
- builder 擁有 source participation 與 source `moduleKey` / internal tag 的選擇權
- rag 只處理已被選入的 source 補充內容

## Java Compatibility Rules
文件提到「與 Java 一致」時，應以 Java 現行 code 為準。

第一版應優先保住：
- public/admin HTTP API routes
- request/response contract
- error codes
- prompt assembly order
- graph save/load semantics
- template CRUD semantics
- output behavior
- override behavior

### Compatibility decisions that are still open
目前仍待定的項目應明確標示為 open question，不要提前寫死。

例如：
- `ProfileConsult` contract 的後續演進時程
- 是否保留 `aiagent[]` 舊 graph request 形狀
- dynamic/vector retrieval 的 backing store 細節
- HTTP router / XLSX library 選型

## What Light DDD Means In This Project
本專案的輕 DDD 意思是：
- 用業務模組切 package
- 保留清楚的責任邊界
- UseCase 對應案例
- Service 對應規則
- 不做過重抽象

不代表：
- 一定拆三套 model
- 一定做 domain event
- 一定做 full hexagonal / CQRS ceremony

## Development Checklist
每次新增或修改功能前，至少檢查：

1. 這次變更對應哪個 actor 與哪個 scenario
2. 這次變更的 acceptance criteria 是否已寫進文件
3. 這個變更屬於哪個 module
4. 這個變更屬於哪一層
5. 是 HTTP contract 還是 gRPC contract
6. 是否牽涉 `analysisPayloads` / `subjectProfile` / `analysisType` / `moduleKey`
7. 是否先寫 UseCase 測試
8. 是否有需要補 Service 或 grpcapi 測試
9. 是否影響 Java 相容行為
10. 是否影響 Firestore document shape
11. 文件是否需要同步更新

## Final Rule
不要依賴「下次應該看得懂」這種假設。

如果某個規則：
- 容易忘
- 容易誤解
- 容易讓 AI 產生歧義

就應該把它寫進文件，而不是留在腦中。
