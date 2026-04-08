# PromptGuard Module Spec

## Purpose
這份文件定義 `promptguard` module 的第一版規格。

這個 module 的目標是先做 prompt injection / override 風險判定，再依判定結果決定：
- 直接放行
- 直接擋下
- 升級到 LLM guard 做第二層判定

這份文件只描述目前已確認的模組邊界、分層、方法責任與啟動切換方式。
更細的文本規則、實際分數算法與 LLM prompt 內容，留待後續討論。

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
   ├─ cloud gemma
   └─ local gemma
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

## Responsibilities
- 接收 `raw user text`
- 執行第一層文本分數判定
- 將文本判定結果正規化成統一 decision 結構
- 若第一層判定為高風險，直接擋下
- 若第一層判定為 `needs_llm`，轉入 LLM guard
- 依啟動環境變數切換 cloud gemma 或 local gemma
- 回傳統一的 guard evaluation 結果

## Non-Responsibilities
- 不組裝 builder prompt
- 不解析 astrology / mbti / profile payload 業務內容
- 不負責對客最終回答生成
- 不在第一版內實作完整 keyword / regex / scoring rule engine
- 不在第一版內定義完整 LLM prompt 內容

## Public Entry Shape
module 對外應保留單一 usecase 入口。

建議結構：

```text
UseCase
└─ Evaluate(rawUserText)

Service
├─ Evaluate(rawUserText)
├─ ScoreText(rawUserText)
└─ EvaluateWithLLM(rawUserText)
```

說明：
- `UseCase` 對外只暴露單一 `Evaluate`
- `Service` 內部分成三個方法
- 不因為有 text scoring 與 llm guard 兩條路，就拆成兩個 usecase

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
   └─ Step 2: EvaluateWithLLM(rawUserText)
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

## LLM Guard Routing
當 text scoring 回 `needs_llm` 時，應進入第二層 LLM guard。

這一層第一版只要求先切出 provider routing：

```text
llm guard mode
├─ cloud
└─ local
```

第一版先把路由、方法與啟動切換定義清楚；
cloud / local 的實際 request body、prompt 內容與回應解析細節，後續再補。

這版的 placeholder 行為可先固定為：

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

## Startup Configuration Contract
`promptguard` 應使用獨立環境變數，不與主分析模型設定混用。

建議環境變數：

```text
INTERNAL_AI_COPILOT_PROMPTGUARD_MODE
  -> cloud
  -> local

INTERNAL_AI_COPILOT_PROMPTGUARD_MODEL
  -> guard model name

INTERNAL_AI_COPILOT_PROMPTGUARD_BASE_URL
  -> provider endpoint

INTERNAL_AI_COPILOT_PROMPTGUARD_API_KEY
  -> provider credential
```

規則：
- `PROMPTGUARD_MODE=cloud`
  - 代表打 hosted Gemma
  - 通常需要 API key
- `PROMPTGUARD_MODE=local`
  - 代表打 local Gemma / local LLM endpoint
  - API key 可選
- 若 mode 缺失或非法
  - 第一版先 fallback 到 `cloud`

## Boundary Rule
- `promptguard` 只看 `raw user text`
- 不看附件內容
- 不看 builder 組裝後的完整 instructions
- 不負責業務內容分析
- 不回頭介入 builder prompt assembly

## Open Questions
- 未來是否需要保留 `matchedRules[]` 類型欄位
- cloud gemma 與 local gemma 的 request/response contract 是否要共用同一個 parser
- 第二層 LLM guard 的固定 prompt contract 要不要獨立成另一份 module 文件
