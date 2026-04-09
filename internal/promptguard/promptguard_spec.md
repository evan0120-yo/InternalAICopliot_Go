# PromptGuard Module Spec

## Purpose
這份文件定義 `promptguard` module 的第一版規格。

這個 module 的目標是先做 prompt injection / override 風險判定，再依判定結果決定：
- 直接放行
- 直接擋下
- 升級到 LLM guard 做第二層判定

這份文件描述：
- `promptguard` module 自己的分層與方法責任
- `promptguard` 與星座主流程的串接架構
- 第二層 LLM guard 目前應如何向下呼叫 `builder` 與 `aiclient`

更細的文本規則、實際分數算法與最終 LLM prompt wording，留待後續討論。

## Overview

```text
raw user text
│
├─ text scoring
│  ├─ allow
│  ├─ block
│  └─ needs_llm
│
└─ llm guard
   ├─ builder guard prompt assembly
   ├─ aiclient guard analyze
   │  ├─ cloud gemma
   │  └─ local gemma
   └─ return guard json
```

第一版採兩段式判定：

1. 先做 text scoring
2. 若 text scoring 結果是 `needs_llm`，再進入 LLM guard

第一版 text scoring 的真實文本判讀暫不落地。
在這版中，text scoring 先固定回：
- `decision=needs_llm`
- `source=text_rule`
- `reason=TEXT_RULE_PLACEHOLDER`
- `score=50`

## Layering In This Module

```text
UseCase -> Service
```

這個 module 第一版不需要 repository。

和主流程的串接應是：

```text
gatekeeper usecase
  -> promptguard usecase
     -> promptguard service
        -> builder guard prompt assembly
        -> aiclient guard analyze
  -> builder consult main flow
```

規則：
- `gatekeeper service` 只保留原本的 request shape / builder / app / profile normalization 驗證
- `promptguard` 不應直接被塞進 `gatekeeper service`
- `gatekeeper usecase` 才是決定「先做 guard，再進主流程」的入口編排點

## Responsibilities
- 接收 `raw user text`
- 執行第一層文本分數判定
- 將文本判定結果正規化成統一 decision 結構
- 若第一層判定為高風險，直接擋下
- 若第一層判定為 `needs_llm`，轉入 LLM guard
- 在 `needs_llm` 路徑下，自己向下呼叫 `builder` 組 guard prompt
- 在 `needs_llm` 路徑下，自己向下呼叫 `aiclient` 打 guard model
- 依啟動環境變數切換 cloud gemma 或 local gemma
- 回傳統一的 guard evaluation 結果

## Non-Responsibilities
- 不自己硬寫最終 guard prompt 字串
- 不解析 astrology / mbti / profile payload 業務內容
- 不負責對客最終回答生成
- 不在第一版內實作完整 keyword / regex / scoring rule engine
- 不在第一版內把完整主流程 source / rag 載進 guard path
- 不在第一版內重用 main consult 的整份 instructions

## Public Entry Shape
module 對外應保留單一 usecase 入口。

建議結構：

```text
UseCase
└─ Evaluate(command)

Service
├─ Evaluate(command)
├─ ScoreText(rawUserText)
└─ EvaluateWithLLM(command)
```

說明：
- `UseCase` 對外只暴露單一 `Evaluate`
- `Service` 內部分成三個方法
- 不因為有 text scoring 與 llm guard 兩條路，就拆成兩個 usecase
- 第二層 builder / aiclient orchestration 應封裝在 `promptguard service` 內，不回流到 `gatekeeper`

建議 command 至少包含：
- `rawUserText`
- `appId`
- `builderConfig`
  - 供 builder dedicated guard prompt assembly 使用
  - 其中至少要能取到 `builderCode` / `builderName`

## Evaluation Contract
第一版應使用統一 evaluation shape，而不是只回裸分數。

建議欄位：

```text
decision
├─ allow
├─ block
└─ needs_llm

source
├─ text_rule
└─ llm_guard
```

建議回傳內容：
- `decision`
- `score`
- `reason`
- `source`

## Decision Rule

```text
promptguard evaluate
│
├─ Step 1: ScoreText(rawUserText)
│
├─ score decision = block
│  └─ 直接回 block
│
├─ score decision = allow
│  └─ 直接回 allow
│
└─ score decision = needs_llm
   └─ Step 2: EvaluateWithLLM(command)
      │
      ├─ builder 組 guard prompt
      ├─ aiclient 打 guard model
      ├─ parse guard json
      └─ 回統一 evaluation
```

## First-Version Text Scoring Rule
這版 text scoring 的目標不是完成真正規則引擎，而是先把主流程接好。

因此第一版暫定：

```text
ScoreText(rawUserText)
  -> 一律回 needs_llm
  -> score 先回固定 placeholder value = 50
  -> reason = TEXT_RULE_PLACEHOLDER
  -> source = text_rule
```

這代表：
- 第一版不靠 text scoring 做真實攔截
- 第一版先把主 decision flow 與 LLM 接點建立起來
- 真正 keyword / regex / scoring rule 後續再補

## Main-Flow Integration Target
第一版先和星座主流程串。

這裡的「星座主流程」是指：
- `ProfileConsult`
- `PublicProfileConsult`
- `LinkChat / astrology` 這條 structured profile 路徑

第一版不要求：
- generic consult 一起接上
- 所有 builder 一次接上

整體串接點應為：

```text
gatekeeper usecase
│
├─ 先做原本 ValidateProfileConsult / ValidateExternalProfileConsult
│
├─ 再呼叫 promptguard usecase
│  ├─ block -> 直接回 blocked business response
│  ├─ allow -> 繼續主流程
│  └─ needs_llm -> promptguard 內部升級到 LLM guard
│
└─ guard 通過後
   -> builderConsult.Consult
```

## LLM Guard Routing
當 text scoring 回 `needs_llm` 時，應進入第二層 LLM guard。

這一層的責任不在 `gatekeeper`，而在 `promptguard service`。

它應採這條路徑：

```text
promptguard service
│
├─ builder guard prompt assembly
│
└─ aiclient guard analyze
   ├─ cloud
   └─ local
```

規則：
- `gatekeeper` 不應自己去 call `builder` 與 `aiclient`
- `promptguard service` 應完整包住第二層 LLM guard orchestration
- `builder` 在這裡只負責 guard prompt assembly
- `aiclient` 在這裡只負責依 env 切 cloud/local model

未 wiring builder 或 llm route 時，placeholder 行為維持：

```text
cloud route
  -> decision = needs_llm
  -> score = 50
  -> reason = LLM_GUARD_CLOUD_PLACEHOLDER
  -> source = llm_guard

local route
  -> decision = needs_llm
  -> score = 50
  -> reason = LLM_GUARD_LOCAL_PLACEHOLDER
  -> source = llm_guard
```

完整 wiring 後的 current path 應為：

```text
promptguard service
  -> builder dedicated guard prompt assembly
  -> aiclient AnalyzeGuard
  -> parse dedicated guard JSON
  -> status=true  -> decision=allow
  -> status=false -> decision=block
```

## Guard Prompt Assembly Boundary
第二層 LLM guard 需要 `builder` 參與，但不是走主 consult prompt。

應採 dedicated guard prompt assembly：

```text
builder guard prompt
├─ raw user text
├─ builder identity
├─ app identity
├─ consult mode
└─ optional analysis type summary
```

第一版不應帶入：
- source prompts
- rag contents
- attachments
- 完整 main consult instructions
- astrology / mbti 業務分析內容

理由：
- prompt injection 的判斷目標是 `raw user text`
- `source / rag` 屬於業務語意材料，不是安全判斷材料
- 把完整 source / rag 帶進 guard path，會把安全判斷和業務分析混在一起

## Guard LLM Return Contract
第二層 LLM guard 應回自己的 JSON，不直接沿用主 consult response schema。

建議 contract：

```json
{
  "status": true,
  "statusAns": "SAFE",
  "reason": "internal guard note"
}
```

block example：

```json
{
  "status": false,
  "statusAns": "prompts有違法注入內容",
  "reason": "internal guard note"
}
```

解析規則：
- `status=true` 代表 allow
- `status=false` 代表 block
- schema / transport / provider failure 應視為 system error，而不是 business block

## Blocked Response Handling
當 `promptguard` 判定 block 時，應回到 `gatekeeper usecase` 做主流程攔截。

規則：
- block 應回正常 business response
- 不應把 prompt injection block 當成 HTTP 4xx validation error
- 真正要拋錯的是：
  - guard prompt assembly 失敗
  - aiclient provider 失敗
  - guard json parse 失敗
  - config 缺失或 provider 不支援

## Startup Configuration Contract
`promptguard` 的主要啟動方式應改為跟主分析共用同一個 numeric profile。

主要環境變數：

```text
INTERNAL_AI_COPILOT_AI_PROFILE
GEMINI_API_KEY
OPENAI_API_KEY
```

`AI_PROFILE` 對 promptguard 的映射：

```text
1 -> cloud
2 -> cloud
3 -> cloud
4 -> cloud
5 -> cloud
6 -> local
7 -> local
```

規則：
- profile `1~5`
  - 代表打 hosted Gemma
  - 讀 `GEMINI_API_KEY`
- profile `6~7`
  - 代表打 local Gemma / local LLM endpoint
  - 預設 base URL 為 `http://localhost:11434`
- `GEMINI_API_KEY` 是 promptguard cloud 的主要 credential
- 若同時存在 `GEMINI_API_KEY` 與舊的 `INTERNAL_AI_COPILOT_PROMPTGUARD_API_KEY` / `INTERNAL_AI_COPILOT_GEMMA_API_KEY`，runtime 會優先採用 `GEMINI_API_KEY`
- 舊的 `INTERNAL_AI_COPILOT_PROMPTGUARD_*` 與主 Gemma 相容 env 仍保留 fallback，但只作相容用途，不建議日常手設

## Boundary Rule
- `promptguard` 只看 `raw user text`
- 不看附件內容
- 不看 builder 組裝後的完整 instructions
- 不負責業務內容分析
- 不回頭介入 main consult prompt assembly
- 一旦 promptguard 正式承接 text injection / override guard，main consult prompt 不應再重複這段判定規則
- 第二層 builder 只組 dedicated guard prompt
- 第一版不載 source / rag

## Open Questions
- 未來是否需要保留 `matchedRules[]` 類型欄位
- cloud gemma 與 local gemma 的 request/response contract 是否要共用同一個 parser
- 第二層 LLM guard 的固定 prompt contract 要不要獨立成另一份 module 文件
