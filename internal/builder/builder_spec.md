# Builder Module Spec

## Purpose
這份文件是 builder module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Builder 是 Go 版後端的編排中心。它接收 Gatekeeper 傳來的 consult 請求，載入 builder/source，呼叫 rag module resolve 補充內容，組裝 prompt，再交給 aiclient 與 output。

同時，builder 也負責 graph 與 template 相關 API 的業務邏輯。

在 LinkChat profile-analysis 這條線上，builder 會保留單一 builder 作為整體 source/rag 骨架，並在 app-aware strategy 內再做第二層 analysis factory 分流，例如 `astrology`、`mbti`。

在第一版 promptguard integration 中，builder 也會提供一條 dedicated guard prompt assembly 能力，供 promptguard module 呼叫；這條 path 只負責組 prompt，不負責 allow/block 決策。

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
- 對 promptguard 暴露 dedicated guard prompt assembly 能力

### Service responsibilities
- prompt assembly
- dedicated promptguard prompt assembly
- app-aware prompt assembly strategy dispatch
- override strategies
- graph normalize / merge / validation rules
- template reorder / validation rules
- module-aware source normalization
- structured subject profile rendering
- theory-translation-backed profile transformation
- 依任務類型 / builderCode 決定 AI route code

### Repository responsibilities
- builder/source/template persistence
- Firestore query/write/batch mapping

## ConsultMode
builder consult command 必須帶明確 mode：
- `ConsultModeGeneric`
- `ConsultModeProfile`
- `ConsultModeExtract`

規則：
- `ConsultModeGeneric` 代表 generic consult，走全量 source 選取規則。
- `ConsultModeProfile` 代表 profile consult，走 app-aware structured profile 規則。
- `ConsultModeExtract` 代表 extraction 類任務，例如 LineBot 備忘錄 / 日程事件抽取。
- builder 不得靠 structured profile payload 是否為空推斷 mode。
- `subjectProfile` 缺值且 `userText!=""` 或 `intentText!=""` 在 `ConsultModeProfile` 中仍是合法 request。

補充方向：
- 若未來新增 LineBot extraction 或其他非 profile 任務，builder 應透過新的 task kind / consult mode / dedicated request contract 承接，而不是硬塞進現有 profile shape。

## Builder Factory
`ConsultUseCase` 不應長期直接承擔所有任務分支；任務級差異應由 builder factory 選出的 task builder 承接。

```text
ConsultUseCase
└─ BuilderFactory
   ├─ GenericTaskBuilder
   ├─ ProfileTaskBuilder
   └─ ExtractTaskBuilder
      │
      ├─ prepare sources
      ├─ assemble prompt materials
      ├─ choose response contract
      └─ choose AI route code
```

規則：
- `ConsultUseCase` 應保留共享 orchestration：
  - 載入 builder / source
  - resolve RAG
  - call aiclient
  - call output
- task builder 應承接任務級差異：
  - source 前處理
  - prompt 組裝
  - response contract
  - route selection
- 新任務優先透過新增 task builder 擴充，而不是持續擴大 `ConsultUseCase` 內的條件分支。

## AI Route Selection Ownership
builder 應擁有 AI route 選擇權；aiclient 只負責執行 builder 指定的 route。

```text
builder
├─ BuilderFactory
│  ├─ GenericTaskBuilder
│  ├─ ProfileTaskBuilder
│  └─ ExtractTaskBuilder
├─ task builder 載入 source / rag / template
├─ task builder 準備 instructions / user message / schema
└─ task builder 選 AI route code
   ├─ direct_gemma
   ├─ direct_gpt54
   └─ gemma_then_gpt54
      │
      ▼
   aiclient
   ├─ factory 選 executor
   └─ executor 真正與 AI 溝通
```

規則：
- builder 的責任是準備素材與決定 route code。
- builder 不應承擔多階段 AI stage transition 的交互邏輯。
- aiclient 不應自行從 builderCode、subjectProfile 或 prompt 內容猜測要打哪個 model。
- route code 在 code 裡應使用明確 enum / constant，不應散落裸數字。
- 若未來新增新的 AI 溝通方式，builder 的主要變更應收斂成新增 route selection，而不是重改整條 consult orchestration。

route example：
- `direct_gemma`
  - 直接打 Gemma
- `direct_gpt54`
  - 直接打 GPT-5.4
- `gemma_then_gpt54`
  - 先打 Gemma，再由 aiclient executor 決定後續 GPT-5.4 互動流程

## Extraction Prompt Assembly
LineBot extraction 這條線不應重用 `ProfileConsult` prompt shape；builder 應提供 extraction 專用的素材組裝方式。

```text
ConsultModeExtract
├─ builderCode = line-memo-crud
├─ input
│  ├─ messageText
│  ├─ referenceTime
│  └─ timeZone
├─ route = direct_gemma
└─ prompt blocks
   ├─ [TASK]
   ├─ [REFERENCE_TIME]
   ├─ [TIME_ZONE]
   ├─ [INPUT_TEXT]
   ├─ [TIME_RULES]
   └─ [OUTPUT_SCHEMA]
```

規則：
- 第一版 `ConsultModeExtract` 先以 `builderCode=line-memo-crud` 為主要任務。
- extraction path 不應帶 `subjectProfile`、`[REQUEST_INTENT]` 或 profile common source blocks。
- builder 應把 `referenceTime` 與 `timeZone` 明確寫進 prompt，讓 Gemma 可把相對時間轉成絕對時間。
- builder 應把預設時間規則明寫進 prompt：
  - 未指定結束時間 -> `endAt = startAt + 30 分鐘`
  - 只有日期、未指定開始時間 -> `00:00:00 ~ 01:00:00`
- builder 應要求 AI 只回最小 extraction schema，不應要求 AI 回 `taskCode`、`appId`、`builderCode`、`requestId` 或 `rawText`。

第一版最小 schema：

```text
extraction result
├─ operation
├─ summary
├─ startAt
├─ endAt
├─ location
└─ missingFields[]
```

持久化提醒：
- Firestore 不應存單一 `start ~ end` 字串。
- 下游應拆欄存 `startAt` / `endAt`。

## Profile Input Split
builder 在 profile-analysis path 應接收兩種自然語言輸入：
- `userText`
  - 使用者自由輸入
  - untrusted
  - 上游已先經過 promptguard
- `intentText`
  - 上游系統決定好的任務意圖
  - 設計目標上應是 trusted
  - 但 current runtime 若由 transport 直接帶入，gatekeeper 仍會先經過 promptguard

規則：
- builder 不應自行猜哪段文字是 trusted、哪段是 untrusted
- gatekeeper 應把 `userText` 與 `intentText` 明確分欄傳進 builder consult command
- 相容期內若 gatekeeper 仍傳舊 `text`，應把它視為 `userText`
- builder 的責任是把 `userText` 與 `intentText` 有標記地合併進主 prompt，而不是在 builder 內再做安全分類

## App-Aware Prompt Assembly
builder consult command 應帶 optional `appId`。

規則：
- `appId=""` 時，prompt assembly 應走 default strategy。
- `appId` 有值時，builder 應透過 factory / registry 選出對應的 app-specific strategy。
- strategy 的責任是控制 app-specific 的 profile/context 組法，不應重做整條 consult orchestration。
- framework header、source order、rag order、override 規則與 framework tail 仍屬 shared assembly skeleton。
- strategy interface 應以內部 prompt assembly context 為主，不應直接把 assemble service 內部參數形狀外洩成公共契約。
- 若某個 app 需要更細的欄位/value 語意組裝，應由該 strategy 自己定義 key resolution 規則。
- LinkChat 目前已落地 source graph 欄位（`sourceType` / `matchKey` / `sourceIds[]`），且 rag ownership 與 shared consult skeleton 不被反轉。

## LinkChat Two-Layer Factory
builder 第一層只處理 app-aware strategy；LinkChat 的 analysis-specific 分流留在第二層 factory。

規則：
- 第一層：`default` / `linkchat`
- 第二層僅存在於 `linkchat` strategy 內，由 `analysisType` 分流，例如 `astrology`、`mbti`
- `builderId` 仍決定整體 builder/source/rag 骨架
- external app 不再需要額外傳 top-level `analysisModules`
- source 可以帶 optional `moduleKey` 作為 internal tag，但不再要求由 top-level request list 直接驅動
- source `moduleKey` 缺失或空值時，仍可視為 common source
- LinkChat strategy 可自行決定是否使用 `source.moduleKey`、analysis scope key 或其他 internal key system 來選擇 prompt 片段

## Structured Subject Profile
builder 會收到 external app 已正規化完成的 `subjectProfile`。

builder 的責任是：
- 依 `appId` 與 strategy 將 `subjectProfile` 轉成 deterministic prompt block
- 對 `appId=linkchat` 的 request，先進入 LinkChat strategy，再依 payload 內的 `analysisType` 分派到第二層 analysis factory
- 不自行回 LinkChat 查 subject data
- 不自行補齊被 LinkChat 省略的 analysis payload
- 在 `ConsultModeProfile` 且 `subjectProfile` 為空時，允許 `userText`-only、`intentText`-only、或兩者並存的 profile request 繼續執行

builder 不應做的事：
- 不推測 external app 的 module entitlement
- 不重新判斷 subject 缺資料時應不應送某個 analysis type
- 不要求 external app 直接送 Internal 私有 prompt code
- 不要求不同 analysis type 共用同一套 payload shape

## Canonical Value And Source Resolution
某些 app-specific strategy 可要求 external app 先把 raw value 正規化成 canonical key，再由 Internal 直接依 composable source graph 組出 prompt。

規則：
- LinkChat 應在自己的 DB / backend 先完成 `raw value / alias -> canonical key` 正規化。
- analysis scope key 可由 LinkChat 第二層 factory 從 `analysisType` 映射得出，不要求 external request 直接送 Internal module key。
- slot key 在 composable analysis type 中應視為 stable slot key，用來對應 primary source。
- canonical key 應直接對應某個 fragment source 的 `matchKey`。
- strategy 應以 `slot key -> primary source.matchKey`、`canonical value -> fragment source.matchKey` 做 source lookup。
- 若 payload slot 採 weighted canonical entry 形狀，strategy 應保留 entry 順序與 `weightPercent`，而不是先降成純字串陣列。
- prompt 片段內容應由 source / rag graph 承接，讓 admin graph UI 可以直接編輯。
- 不是每個 analysis type 都需要 canonical-key composable path；未配置此路徑的 analysis type 可保留原始值 render。
- `theoryVersion` 若存在，僅作 external metadata；不是 Internal source lookup 的必要條件。
- AI 最終不應直接看到 raw theory 詞，也不應看到 internal lookup key；應只看到展開後的最終 prompt 內容。
- slot-level 語意標籤（如 `人生主軸`、`情緒本能`）應放在 primary source 的 `prompts` 欄位。

weighted canonical entry example：

```json
{
  "sun_sign": [
    { "key": "capricorn", "weightPercent": 70 },
    { "key": "aquarius", "weightPercent": 30 }
  ],
  "moon_sign": [
    { "key": "pisces" }
  ],
  "rising_sign": [
    { "key": "aquarius" }
  ]
}
```

weighted entry 規則：
- `key` 必填
- 單一 entry 可省略 `weightPercent`
- 同一 slot 若有多個 entries，則每個 entry 都應提供 `weightPercent`
- 同一 slot 若有多個 entries，`weightPercent` 總和應為 `100`

## Composable Source Graph
LinkChat astrology 這條線目前的 source 不再只是一個 flat prompt block；它同時也是可被組合的 prompt node。

source graph 應至少支援：
- `sourceType=primary`
  - 頂層 source
  - 由 LinkChat analysis parser 根據傳入 JSON 的 slot 順序決定要不要進 prompt
- `sourceType=fragment`
  - 可被 primary 或其他 fragment 引用的片段 source
- `matchKey`
  - 給 strategy / canonical key lookup 用來解析 slot 與 value
- `sourceIds[]`
  - 表示 child sources
  - 陣列順序即 child expansion 順序

builder 組裝規則：
```text
LinkChat payload
  -> analysis parser 先決定主順序
     例如 sun -> moon -> rising
  -> 每個主槽位先選 primary source
  -> 再依 canonical value 找到 fragment source
  -> fragment source 若有 sourceIds[]，照填入順序繼續展開
  -> 每個 source 各自再帶自己的 rag children
  -> 最後組成完整 prompt block
```

補充規則：
- primary 順序不是靠 source graph 自己猜，而是由 analysis parser / request JSON 語意決定。
- `sourceIds[]` 的順序應原樣保留，不做額外排序。
- 第一版先不要求防循環與跨鏈去重；若作者配置重複片段，prompt 可重複出現。
- 同一個 fragment source 可被多個 primary sources 重複引用；這在最終 prompt 裡重複出現是合理行為。

## Subject Profile Rendering Rules
- LinkChat 第二層 factory 應將各 analysis payload 轉成 deterministic prompt fragment
- 同一個 `analysisType` 不可重複
- payload 內若有具順序語意的陣列，應依該 analysis factory 的規則保留原序
- payload 內若為 weighted canonical entries，應保留 entry 輸入順序與對應 `weightPercent`
- 若 value 內含 `\`，render 時應 escape 為 `\\`
- 若 value 內含 `|`，render 時應 escape 為 `\|`
- `[SUBJECT_PROFILE]` block 應固定插在 `[RAW_USER_TEXT]` 後、第一個 source block 前
- LinkChat strategy 應直接以 canonical value 對 `source.matchKey` 做 lookup，再把展開後的最終語意片段組進 `[SUBJECT_PROFILE]`
- 若同一個 slot 命中多個 weighted entries，render 應將百分比標在展開後的語意片段前，而不是暴露 raw canonical key
- 不再要求 `[THEORY_CODEBOOK]` 作為 shared prompt block 的一部分暴露給 AI

## Subject Profile Prompt Format
default strategy 採 markdown section 風格；LinkChat strategy 可依 analysis payload 自行組裝 deterministic block：

```text
## [SUBJECT_PROFILE]
### [analysis:astrology]
... LinkChat astrology factory 組出的 deterministic block ...

### [analysis:mbti]
... LinkChat mbti factory 組出的 deterministic block ...
```

weighted render 範例：

```text
### [analysis:astrology]
主執行緒, 發展有好有壞, 主導做事方式和習慣, 以及思維output框架:
70% <capricorn 展開後的最終語意片段>
30% <aquarius 展開後的最終語意片段>
```

規則：
- block 命名與內容可由 app-specific strategy 決定，但必須 deterministic
- 若沒有 `subjectProfile`，則不產生此 block
- app-specific strategy 可改變這段 block 的呈現形式，但 shared prompt skeleton 不變
- 若 analysis type 走 canonical key composable path，Internal 應直接展開 source graph，不要求額外 expose `THEORY_CODEBOOK`

## Runtime Cache Limitation
- app prompt strategy registry 目前可在 runtime 做 process-local cache
- 第一版不要求 TTL 或主動 invalidation
- 若 Firestore 中的策略設定或 source graph 被修改，服務需重啟後才保證讀到最新資料

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
- dedicated guard prompt assembly

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
- LinkChat strategy 若要用 `analysisType`、slot key、value key 做更細的語意片段組裝，應在 strategy 內自己查表與拼接
- source graph 若新增 `sourceIds[]`，代表 source 可以再引用其他 source；這不改變 rag 屬於 source 的關係
- `source.moduleKey` 若存在，僅作 internal tag；它可以被 LinkChat strategy 使用，也可以被忽略，不需要升級成新的 shared request contract
- promptguard path 不走 rag resolution；promptguard 不應為了 injection 判斷額外讀取 source / rag

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
  -> choose AI route code
  -> run app-aware prompt strategy inside assemble prompt
  -> append app-specific profile/context block when present
  -> assemble prompt
  -> ai client analyze by selected route
  -> output render
```

補充：
- `choose AI route code` 應發生在 builder 內。
- aiclient 接到的應是「素材 + route code」，不是再回頭看 builder 內容自行猜 provider。

## PromptGuard Prompt Assembly Boundary
第一版 promptguard integration 不應重用整條 main consult prompt assembly。builder 應提供 dedicated guard prompt assembly path，讓 promptguard service 可在需要 LLM guard 時取得專用 prompt。

最小必要輸入應以 guard context 為主，例如：
- `appId`
- `builderId`
- `builderCode`
- `builderName`
- `ConsultModeProfile`
- analysis summary（若此 builder 需要最小 analysis hint）
- 單一 candidate text

規則：
- builder 在 promptguard path 只負責組出 deterministic guard prompt，不直接做 allow/block 決策。
- 第一版 promptguard path 不應載入或展開：
  - 另一段 profile text
  - source prompts
  - rag contents
  - attachments
  - full main consult instructions
  - `[SUBJECT_PROFILE]` 主分析內容
- promptguard path 應只搬移 prompt injection / override 判定所需的 guard policy，不應把 main consult 的回覆風格、附件失敗說明、輸出格式要求整段搬進來。
- promptguard 成為唯一的 `userText` injection / override 判定承接者後，main consult prompt 不應再重複這些 guard clauses。

## Prompt Assembly
第一版目標與 Java 一致，並加上 app-aware structured profile/context block：

```text
Prompt 組裝資料管線（由上至下依序輸出）

  ┌─────────────────────────────────────────────────┐
  │ 1. FRAMEWORK_HEADER                             │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 2. [REQUEST_INTENT]（optional）                 │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 3. [RAW_USER_TEXT]（optional）                  │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 4. App-Aware Profile/Context Block              │
  │    （僅當 structured subjectProfile 存在時插入） │
  │    └── [SUBJECT_PROFILE]                        │
  │       （含 Internal 已翻譯好的語意片段）         │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 5. Selected Sources（依 orderNo ASC）           │
  │    ├── source[0]                                │
  │    │     └── 6. Resolved RAG[0]                 │
  │    ├── source[1]                                │
  │    │     └── 6. Resolved RAG[1]                 │
  │    └── source[N]                                │
  │          └── 6. Resolved RAG[N]                 │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 7. [USER_INPUT]（optional）                     │
  │    僅當沒有 override 消化 userText 時才附加     │
  └──────────────────────┬──────────────────────────┘
                         ↓
  ┌─────────────────────────────────────────────────┐
  │ 8. [FRAMEWORK_TAIL]                             │
  │    安全與 JSON 回應契約保底                     │
  └─────────────────────────────────────────────────┘
```

### Important Behavior
- `systemBlock=true` 是資料層區塊標記
- 最後的安全與 JSON 回應契約仍由 `FRAMEWORK_TAIL` 保底
- `intentText` 與 `userText` 應以不同區塊進 prompt，不應直接事先拼成一個無標記字串
- 若 `intentText` 有值，builder 應輸出 `[REQUEST_INTENT]` 區塊
- 若 `userText` 有值，builder 應輸出 `[RAW_USER_TEXT]` 區塊
- 若 overridable RAG 已經消化 `userText`，則不再附加 `[USER_INPUT]` 區塊
- default 與 app-specific strategy 都必須共用相同的 framework header / source / rag / tail 順序

## Ordering Rules
- generic/default path：
  - source order: `orderNo ASC`, tie-break with `sourceId`
  - rag order: `orderNo ASC`
- LinkChat composable source graph path：
  - primary source 順序：由 analysis parser / request JSON 語意決定
  - child fragment 順序：由 `sourceIds[]` 填入順序決定
  - rag order：仍為 `orderNo ASC`
- graph save 時，非系統 primary source 與其 rag configs 可重編 canonical order
- 若 source graph 引入 fragment source，`sourceIds[]` 順序不應被 graph save 自動改寫

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
- LinkChat 送 canonical value（必要時可附帶 `theoryVersion` metadata），而不是 Internal 私有 prompt code
- builder 依 `builderId` 載入整體 source/rag 骨架
- builder 依 `appId` 選第一層 prompt strategy，LinkChat 再依 `analysisType` 選第二層 factory
- builder 在需要時直接以 canonical value 做 fragment lookup，再沿 `sourceIds[]` 展開 composable source graph
- source 若帶 `tags[]`，僅供 admin / 維護者搜尋與分群，不參與 runtime source lookup
- 若 LinkChat 需要用自己的 key system 做欄位/value 級別的語意組裝，應在 LinkChat strategy 內完成；default strategy 與 shared consult skeleton 不受影響
- rag 只處理已被選入的 source 補充資料
- `userText`-only 或 `intentText`-only profile request 仍屬於 profile mode，不屬於 generic consult
- promptguard 第一版只先掛在這條 profile astrology 主流程；builder 在這條線上只提供 dedicated guard prompt assembly，不擴成完整第二條 consult orchestration
