# Builder Module Spec

## Purpose
這份文件是 builder module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Builder 是 Go 版後端的編排中心。它接收 Gatekeeper 傳來的 consult 請求，載入 builder/source，呼叫 rag module resolve 補充內容，組裝 prompt，再交給 aiclient 與 output。

同時，builder 也負責 graph 與 template 相關 API 的業務邏輯。

在 LinkChat profile-analysis 這條線上，builder 會以單一 builder 承接多種動態成長的 analysis modules，並依 request 中的 `analysisModules` 決定要載哪些 source blocks。

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
- 依 `ConsultMode` 與 `analysisModules` 做 source selection
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
- `ConsultModeProfile` 代表 profile consult，走 module-aware source 選取規則。
- builder 不得靠 `analysisModules` 是否為空推斷 mode。
- `analysisModules=[] && text!=""` 在 `ConsultModeProfile` 中仍是合法 request，且只應跑 common sources。

## App-Aware Prompt Assembly
builder consult command 應帶 optional `appId`。

規則：
- `appId=""` 時，prompt assembly 應走 default strategy。
- `appId` 有值時，builder 應透過 factory / registry 選出對應的 app-specific strategy。
- strategy 的責任是控制 app-specific 的 profile/context 組法，不應重做整條 consult orchestration。
- framework header、source order、rag order、override 規則與 framework tail 仍屬 shared assembly skeleton。
- strategy interface 應以內部 prompt assembly context 為主，不應直接把 assemble service 內部參數形狀外洩成公共契約。

## Dynamic Module Selection
單一 builder 可以承接多種 profile-analysis 組合。

規則：
- 只有 `ConsultModeProfile` 會使用 `analysisModules`
- source 可以帶 optional `moduleKey`
- source `moduleKey` 缺失或空值，代表 common source，永遠參與
- source `moduleKey` 有值時，只有當它出現在 `analysisModules` 時才參與本次 profile consult
- `analysisModules` 的順序不帶業務優先權；最終 prompt 順序仍由 source `orderNo` 決定
- `analysisModules` 內不應出現保留字 `common`

## Structured Subject Profile
builder 會收到 external app 已正規化完成的 `subjectProfile`。

builder 的責任是：
- 依 `appId` 與 strategy 將 `subjectProfile` 轉成 deterministic prompt block
- 不自行回 LinkChat 查 subject data
- 不自行補齊被 LinkChat 刪掉的 modules
- 在 `ConsultModeProfile` 且 `subjectProfile` 為空時，允許 text-only profile request 繼續執行

builder 不應做的事：
- 不推測 external app 的 module entitlement
- 不重新判斷 subject 缺資料時應不應送某個 module
- 不要求 external app 直接送 Internal 私有 prompt code

## Theory Mapping And Codebook
某些 app-specific strategy 可要求在 render prompt 前，先將 external app 傳來的 raw value 轉成 Internal private code。

規則：
- mapping key 應至少包含 `appId`、`moduleKey`、`theoryVersion`、`factKey` 與 raw value。
- `factKey` 在 codebook-enabled module 中應視為對應理論 slot 的 stable key。
- 不是每個 module 都需要 code mapping；未配置 mapping 的 module 應保留原始值 render。
- LinkChat 目前只有 `astrology` 視為 codebook-enabled module，必須帶 `theoryVersion`；`mbti` 目前維持 raw render。
- codebook data 應由 infra / repository 提供，strategy 只負責使用，不負責持有 source of truth。

## Subject Profile Rendering Rules
- `modulePayloads[]` 依 `moduleKey ASC` 排序
- `facts[]` 依 `factKey ASC` 排序
- `values[]` 保持 request 原序
- 同一個 `moduleKey` 不可重複
- 同一個 module 內的 `factKey` 不可重複
- 若 value 內含 `\`，render 時應 escape 為 `\\`
- 若 value 內含 `|`，render 時應 escape 為 `\|`
- `[SUBJECT_PROFILE]` block 應固定插在 `[RAW_USER_TEXT]` 後、第一個 source block 前
- LinkChat strategy 的 `[THEORY_CODEBOOK]` 應緊接在 `[SUBJECT_PROFILE]` 後、第一個 source block 前，讓 source prompts 可直接引用解碼規則

## Subject Profile Prompt Format
default strategy 採 markdown section 風格：

```text
## [SUBJECT_PROFILE]
subject: user-123

### [module:astrology]
moon_sign: Pisces
sun_sign: Scorpio

### [module:mbti]
cognitive_stack: Ni|Te|Fi|Se
type: INTJ
```

規則：
- module section 依 `moduleKey ASC`
- fact line 依 `factKey ASC`
- 多值以 `|` 連接，並保留原始順序
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
- source `moduleKey` 選擇
- prompt assembly
- overall consult orchestration

rag 負責：
- 根據 `retrievalMode` resolve rag config
- 對 builder 回傳 resolved content

設計原則：
- builder owns order
- builder owns mode and module selection
- rag owns resolution

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
       filter sources by moduleKey using analysisModules
       (empty analysisModules means common sources only)
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

1. framework header
2. `[RAW_USER_TEXT]`
3. app-aware profile/context block when structured subject profile exists
4. each selected source by `orderNo`
5. each selected source's resolved RAG by `orderNo`
6. optional `[USER_INPUT]`
7. `[FRAMEWORK_TAIL]`

### Important Behavior
- `systemBlock=true` 是資料層區塊標記
- 最後的安全與 JSON 回應契約仍由 `FRAMEWORK_TAIL` 保底
- 若 overridable RAG 已經消化 user text，則不再附加 `[USER_INPUT]` 區塊
- `analysisModules` 只決定哪些 source blocks 參與，不改變 source 的既有 order 規則
- default 與 app-specific strategy 都必須共用相同的 framework header / source / rag / tail 順序

## Ordering Rules
- source order: `orderNo ASC`, tie-break with `sourceId`
- rag order: `orderNo ASC`
- graph save 時，非系統 source 與其 rag configs 可重編 canonical order

## Override
第一版以 Java 現行行為為準。

## Graph Save Semantics
- builder config 採 merge
- 非系統 sources 採 replace
- 既有 `systemBlock=true` source 與其 rag configs 保留
- payload 內的 system source 不直接信任
- 目前仍支援 Java legacy `aiagent[]` 輸入形狀，進入 service 後會先轉成 `sources`
- source 可帶 optional `moduleKey`
- `moduleKey` 在 graph save 時應做 `trim + lowercase`
- `moduleKey` 若為 `common`、空值或缺值，應正規化為缺值 / 空值
- `moduleKey` 若不符合 `^[a-z0-9][a-z0-9_-]*$`，應拒絕儲存

## Template Rules
- template 留在 builder module 內
- `templateKey` 必須唯一
- `orderNo` 需維持 canonical order
- delete template 時需清除 source 上的 template 引用
- `templateName` 必填
- template / graph rag 的 `retrievalMode` 目前只接受 `full_context`

## Boundary Notes For LinkChat Profile Analysis
- LinkChat 決定本次實際送來哪些 modules
- LinkChat 送理論 raw values 與 `theoryVersion`，而不是 Internal 私有 prompt code
- builder 決定這些 modules 對應哪些 source prompts
- builder 依 `appId` 選 prompt strategy，並在需要時透過 codebook 將 raw values 轉成 Internal private code
- rag 只處理已被選入的 source 補充資料
- text-only profile request 仍屬於 profile mode，不屬於 generic consult
