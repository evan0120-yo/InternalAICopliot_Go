# Builder Module Spec

## Purpose
這份文件是 builder module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Builder 是 Go 版後端的編排中心。它接收 Gatekeeper 傳來的 consult 請求，載入 builder/source，呼叫 rag module resolve 補充內容，組裝 prompt，再交給 aiclient 與 output。

同時，builder 也負責 graph 與 template 相關 API 的業務邏輯。

在 LinkChat profile-analysis 這條線上，builder 會保留單一 builder 作為整體 source/rag 骨架，並在 app-aware strategy 內再做第二層 analysis factory 分流，例如 `astrology`、`mbti`。

對應 Java：
- `com.citrus.internalaicopilot.builder`
- `com.citrus.internalaicopilot.source`
- template 相關邏輯

## Layering In This Module

```text
UseCase -> Service -> Repository
```

此模組通常沒有自己的 HTTP handler；handler 由 gatekeeper 或 admin API 入口承接。

## Responsibilities

### UseCase responsibilities
- consult orchestration
- graph use cases
- template use cases
- cross-module coordination with rag / aiclient / output
- concurrency orchestration with `context` + goroutine / wait coordination
- 依 `ConsultMode` 與 app-specific strategy 做 source selection / prompt assembly
- 傳遞 `appId` 到 prompt assembly 階段

### Service responsibilities
- prompt assembly
- app-aware prompt assembly strategy dispatch
- override strategies
- graph normalize / merge / validation rules
- template reorder / validation rules
- module-aware source normalization
- structured subject profile rendering
- codebook-backed profile transformation

### Repository responsibilities
- builder/source/template persistence
- Firestore query/write/batch mapping

## ConsultMode
builder consult command 必須帶明確 mode：
- `ConsultModeGeneric`
- `ConsultModeProfile`

規則：
- `ConsultModeGeneric` 代表 generic consult，走全量 source 選取規則。
- `ConsultModeProfile` 代表 profile consult，走 app-aware structured profile 規則。
- builder 不得靠 structured profile payload 是否為空推斷 mode。
- `subjectProfile` 缺值且 `text!=""` 在 `ConsultModeProfile` 中仍是合法 request。

## App-Aware Prompt Assembly
builder consult command 應帶 optional `appId`。

規則：
- `appId=""` 時，prompt assembly 應走 default strategy。
- `appId` 有值時，builder 應透過 factory / registry 選出對應的 app-specific strategy。
- strategy 的責任是控制 app-specific 的 profile/context 組法，不應重做整條 consult orchestration。
- framework header、source order、rag order、override 規則與 framework tail 仍屬 shared assembly skeleton。
- strategy interface 應以內部 prompt assembly context 為主，不應直接把 assemble service 內部參數形狀外洩成公共契約。
- 若某個 app 需要更細的欄位/value 語意組裝，應由該 strategy 自己定義 key resolution 規則，不應回頭改 shared source / rag 的主資料模型。

## LinkChat Two-Layer Factory
builder 第一層只處理 app-aware strategy；LinkChat 的 analysis-specific 分流留在第二層 factory。

規則：
- 第一層：`default` / `linkchat`
- 第二層僅存在於 `linkchat` strategy 內，由 `analysisType` 分流，例如 `astrology`、`mbti`
- `builderId` 仍決定整體 builder/source/rag 骨架
- external app 不再需要額外傳 top-level `analysisModules`
- source 可以帶 optional `moduleKey` 作為 internal tag，但不再要求由 top-level request list 直接驅動
- source `moduleKey` 缺失或空值時，仍可視為 common source
- LinkChat strategy 可自行決定是否使用 `source.moduleKey`、codebook scope key 或其他 internal key system 來選擇 prompt 片段

## Structured Subject Profile
builder 會收到 external app 已正規化完成的 `subjectProfile`。

builder 的責任是：
- 依 `appId` 與 strategy 將 `subjectProfile` 轉成 deterministic prompt block
- 對 `appId=linkchat` 的 request，先進入 LinkChat strategy，再依 payload 內的 `analysisType` 分派到第二層 analysis factory
- 不自行回 LinkChat 查 subject data
- 不自行補齊被 LinkChat 省略的 analysis payload
- 在 `ConsultModeProfile` 且 `subjectProfile` 為空時，允許 text-only profile request 繼續執行

builder 不應做的事：
- 不推測 external app 的 module entitlement
- 不重新判斷 subject 缺資料時應不應送某個 analysis type
- 不要求 external app 直接送 Internal 私有 prompt code
- 不要求不同 analysis type 共用同一套 payload shape

## Theory Mapping And Codebook
某些 app-specific strategy 可要求在 render prompt 前，先將 external app 傳來的 raw value 轉成 Internal private code。

規則：
- mapping key 應至少包含 `appId`、analysis scope key、`theoryVersion`、slot key 與 raw value。
- analysis scope key 可由 LinkChat 第二層 factory 從 `analysisType` 映射得出，不要求 external request 直接送 Internal module key。
- slot key 在 codebook-enabled analysis type 中應視為 stable theory slot key。
- 不是每個 analysis type 都需要 code mapping；未配置 mapping 的 analysis type 應保留原始值 render。
- LinkChat 目前只有 `astrology` 視為 codebook-enabled analysis type，必須帶 `theoryVersion`；`mbti` 目前維持 raw render。
- codebook data 應由 infra / repository 提供，strategy 只負責使用，不負責持有 source of truth。

## Subject Profile Rendering Rules
- LinkChat 第二層 factory 應將各 analysis payload 轉成 deterministic prompt fragment
- 同一個 `analysisType` 不可重複
- payload 內若有具順序語意的陣列，應依該 analysis factory 的規則保留原序
- 若 value 內含 `\`，render 時應 escape 為 `\\`
- 若 value 內含 `|`，render 時應 escape 為 `\|`
- `[SUBJECT_PROFILE]` block 應固定插在 `[RAW_USER_TEXT]` 後、第一個 source block 前
- LinkChat strategy 的 `[THEORY_CODEBOOK]` 應緊接在 `[SUBJECT_PROFILE]` 後、第一個 source block 前，讓 source prompts 可直接引用解碼規則

## Subject Profile Prompt Format
default strategy 採 markdown section 風格；LinkChat strategy 可依 analysis payload 自行組裝 deterministic block：

```text
## [SUBJECT_PROFILE]
subject: user-123

### [analysis:astrology]
... LinkChat astrology factory 組出的 deterministic block ...

### [analysis:mbti]
... LinkChat mbti factory 組出的 deterministic block ...
```

規則：
- block 命名與內容可由 app-specific strategy 決定，但必須 deterministic
- 若沒有 `subjectProfile`，則不產生此 block
- app-specific strategy 可改變這段 block 的呈現形式，但 shared prompt skeleton 不變
- `THEORY_CODEBOOK` 若存在，其位置屬於 shared profile/context block 的一部分，不應挪到 source blocks 之後

## Runtime Cache Limitation
- app prompt strategy registry 與 theory mapping table 目前可在 runtime 做 process-local cache
- 第一版不要求 TTL 或主動 invalidation
- 若 Firestore 中的策略設定或 theory mapping 被修改，服務需重啟後才保證讀到最新資料

## Why Source Lives Here
- source 沒有獨立對外 use case
- consult / graph 都以 builder 為起點
- Firestore 下，builder + sources 更適合 aggregate 設計

所以 Go 版不再保留獨立 `source` module，但 `source` 概念仍存在於 builder domain 內。

## Builder / RAG Boundary
builder 負責：
- source / rag 順序
- app-aware strategy 與 analysis factory 的 prompt assembly
- prompt assembly
- overall consult orchestration

rag 負責：
- 根據 `retrievalMode` resolve rag config
- 對 builder 回傳 resolved content

設計原則：
- builder owns order
- builder owns mode and app-specific routing
- rag owns resolution

補充：
- source 仍是主 prompt 骨架
- rag 仍是 source 底下的補充內容
- LinkChat strategy 若要用 `analysisType`、slot key、value key、`theoryVersion` 做更細的語意片段組裝，應在 strategy 內自己查表與拼接，不代表 source / rag 要改成以這些 key 為主鍵
- `source.moduleKey` 若存在，僅作 internal tag；它可以被 LinkChat strategy 使用，也可以被忽略，不需要升級成新的 shared request contract

## Use Cases In This Module
- `ConsultUseCase`
- `LoadGraphUseCase`
- `SaveGraphUseCase`
- `ListBuilderTemplatesUseCase`
- `ListAllTemplatesUseCase`
- `CreateTemplateUseCase`
- `UpdateTemplateUseCase`
- `DeleteTemplateUseCase`

這些 use case 應直接對應主要測試案例。

## Consult Flow

```text
Gatekeeper -> ConsultUseCase
  -> load builder config + sources
  -> if ConsultModeGeneric:
       use all eligible sources by order
  -> if ConsultModeProfile:
       keep builder-selected source/rag skeleton
       let app-aware strategy / LinkChat analysis factory decide
       which internal keys or source tags participate
  -> for each selected source with rag configs:
       rag resolver
  -> run app-aware prompt strategy inside assemble prompt
  -> append app-specific profile/context block when present
  -> assemble prompt
  -> ai client analyze
  -> output render
```

## Prompt Assembly
第一版目標與 Java 一致，並加上 app-aware structured profile/context block：

```text
Prompt 組裝資料管線（由上至下依序輸出）

  ┌─────────────────────────────────────────────────┐
  │ 1. FRAMEWORK_HEADER                             │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 2. [RAW_USER_TEXT]                              │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 3. App-Aware Profile/Context Block              │
  │    （僅當 structured subjectProfile 存在時插入） │
  │    ├── [SUBJECT_PROFILE]                        │
  │    └── [THEORY_CODEBOOK]（若 strategy 產生）    │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 4. Selected Sources（依 orderNo ASC）           │
  │    ├── source[0]                                │
  │    │     └── 5. Resolved RAG[0]                 │
  │    ├── source[1]                                │
  │    │     └── 5. Resolved RAG[1]                 │
  │    └── source[N]                                │
  │          └── 5. Resolved RAG[N]                 │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 6. [USER_INPUT]（optional）                     │
  │    僅當沒有 override 消化 user text 時才附加    │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 7. [FRAMEWORK_TAIL]                             │
  │    安全與 JSON 回應契約保底                     │
  └─────────────────────────────────────────────────┘
```

### Important Behavior
- `systemBlock=true` 是資料層區塊標記
- 最後的安全與 JSON 回應契約仍由 `FRAMEWORK_TAIL` 保底
- 若 overridable RAG 已經消化 user text，則不再附加 `[USER_INPUT]` 區塊
- default 與 app-specific strategy 都必須共用相同的 framework header / source / rag / tail 順序

## Ordering Rules
- source order: `orderNo ASC`, tie-break with `sourceId`
- rag order: `orderNo ASC`
- graph save 時，非系統 source 與其 rag configs 可重編 canonical order

## Override
第一版以 Java 現行行為為準。

## Graph Save Semantics

```text
SaveGraph 輸入
     │
     ▼
┌──────────────────────────────────────┐
│ payload 是 legacy aiagent[] 形狀？   │
│   ├── 是 → 轉換為 sources[] 後繼續  │
│   └── 否 → 直接繼續                 │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│ 逐一處理每個 source 的 moduleKey     │
│                                      │
│  moduleKey 值？                      │
│   ├── "common" / 空值 / 缺值         │
│   │     └→ 正規化為缺值（common）    │
│   ├── 符合 ^[a-z0-9][a-z0-9_-]*$    │
│   │     └→ trim + lowercase 後保留   │
│   └── 不符合格式                     │
│         └→ 拒絕儲存                  │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│ 分類處理 sources                     │
│                                      │
│  systemBlock=true？                  │
│   ├── 是（DB 既有）→ 保留原樣        │
│   │   （payload 內的不直接信任）      │
│   └── 否（非系統 source）→ replace   │
│         └→ 以 payload 內容全部取代   │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│ Builder Config → merge（與既有合併） │
└──────────────────┬───────────────────┘
                   ▼
              寫入 DB
```

## Template Rules
- template 留在 builder module 內
- `templateKey` 必須唯一
- `orderNo` 需維持 canonical order
- delete template 時需清除 source 上的 template 引用
- `templateName` 必填
- template / graph rag 的 `retrievalMode` 目前只接受 `full_context`

## Boundary Notes For LinkChat Profile Analysis
- LinkChat 決定本次實際送來哪些 analysis payloads
- LinkChat 送理論 raw values 與 `theoryVersion`，而不是 Internal 私有 prompt code
- builder 依 `builderId` 載入整體 source/rag 骨架
- builder 依 `appId` 選第一層 prompt strategy，LinkChat 再依 `analysisType` 選第二層 factory
- builder 在需要時透過 codebook 將 raw values 轉成 Internal private code
- 若 LinkChat 需要用自己的 key system 做欄位/value 級別的語意組裝，應在 LinkChat strategy 內完成；default strategy 與既有 source / rag 結構不受影響
- rag 只處理已被選入的 source 補充資料
- text-only profile request 仍屬於 profile mode，不屬於 generic consult
