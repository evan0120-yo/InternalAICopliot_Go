# AIClient Module Spec

## Purpose
這份文件是 aiclient module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
AIClient 負責 live AI provider 通訊，以及 local/dev 模式下的 preview 與 mock 輸出。接收 builder usecase 組裝好的 prompt 和附件，依 execution mode 與 provider 選擇正確路徑，最後回傳統一的 business response。

對 LinkChat profile-analysis 這條線來說，AIClient 看到的已經是 builder 組裝完成的 instructions 與 user text，不直接解析 `analysisPayloads`、`subjectProfile` 或理論值本身。

另外保留一個 local/dev 用的 preview mode family：
- 不呼叫外部 AI provider
- 由 backend 決定要回完整 preview 還是只回 prompt body
- 用途是讓前端直接檢查 prompt / user message / 附件摘要長什麼樣，或快速複製主體 prompt 去外部 web GPT 調適

另外保留一個 local/dev 用的 mock mode：
- 不呼叫外部 AI provider
- 由 backend 啟動環境變數顯式控制
- 不再依賴「某家 provider API key 缺值」當成隱式 fallback 條件

## Execution Mode And Provider Selection

目標架構中，aiclient 的判斷分成兩層：

- 第一層：execution mode
- 第二層：live 模式下的 provider routing

```text
execution mode
├─ preview_full
├─ preview_prompt_body_only
└─ live
   ├─ mock mode
   └─ ai provider
      ├─ openai
      └─ gemma
```

其中 execution mode 維持三種：

```text
preview_full
  -> 現在這種完整 preview

preview_prompt_body_only
  -> 只回 builder 已組裝好的 prompt body
  -> 不呼叫外部 AI provider
  -> 不模擬 AI final answer

live
  -> 進入 live path
  -> 若 mock mode 開啟，回 mock analyze
  -> 若 mock mode 關閉，依 provider 呼叫 OpenAI 或 Gemma
```

其中 `preview_prompt_body_only` 的定位要非常明確：

- 它不是 `response only` 的 AI 結果模擬
- 它不是把完整 preview 交給前端後，再由前端自行裁字串
- 它是 backend 直接選擇只輸出 prompt body 的 preview 變體
- 目的主要是讓操作者快速 copy 這段組裝後的主體 prompt，到外部 web GPT 手動調 prompt

而 `mock mode` 的定位也要明確：

- 它不是 preview 的別名
- 它不是 provider credential 缺值時的隱式 fallback
- 它是 backend 啟動時可顯式選擇的 local/dev 執行模式
- 它應保留現在既有的 mock analyze 行為與輸出風格

對應 Java：`com.citrus.internalaicopilot.aiclient`

## Layering In This Module

```text
UseCase -> Service
```

Repository 在第一版通常不是必要的；若未來需要持久化 audit/cache，再另行增加。

## Responsibilities
- 接收 instructions、user text、attachments
- 在 preview mode 下直接回傳完整 AI request preview
- 在 mock mode 下直接回傳 mock analyze 結果
- 在 live mode 下依 provider 路由到 OpenAI 或 Gemma
- 正規化 user text
- 對 provider 需要的附件格式做適配
- 分類附件為 image 或 file
- 呼叫 provider live API
- 解析結構化回應
- 映射 provider 錯誤為 business error
- 將 module-specific profile data 視為 opaque prompt content，而不是在 aiclient 內重做業務判讀

- 若預覽策略為 `preview_prompt_body_only`，aiclient 應只輸出 builder 已組裝好的 prompt body
- aiclient 不應在這個 mode 內合成假的 AI 結果
- mock mode 是否啟用，應由顯式設定決定，而不是由 provider API key 是否存在決定

## Layer Responsibilities

### UseCase
- 作為對 builder 暴露的分析入口
- 負責分析用例的 orchestration

### Service
- execution mode 決策
- mock / live provider 路由
- provider request/response 細節
- temp file / upload / parse 邏輯
- error mapping

## Boundary Rule
- aiclient 不應決定哪些 modules 參與分析
- aiclient 不應理解 LinkChat 的星座、MBTI 或其他理論欄位
- aiclient 只負責把 builder 已組好的內容送到模型
- preview mode 也只回 builder 已組好的內容，不在 aiclient 內重做 prompt 組裝
- `preview_prompt_body_only` 仍遵守同一條邊界：aiclient 只輸出 builder 已組好的 body，不自己重新拼 profile 語意
- provider selection 只處理 AI 通訊差異，不應回頭介入 builder prompt assembly

## Startup Configuration Contract

這條線的啟動配置應明確分離 execution mode、mock mode 與 provider：

```text
INTERNAL_AI_COPILOT_AI_DEFAULT_MODE
  -> preview_full
  -> preview_prompt_body_only
  -> live

INTERNAL_AI_COPILOT_AI_MOCK_MODE
  -> true / false

INTERNAL_AI_COPILOT_AI_PROVIDER
  -> openai / gemma
```

規則：
- `AI_DEFAULT_MODE` 決定 preview family 與 live path
- `AI_MOCK_MODE` 只在 `live` 路徑下生效
- `AI_PROVIDER` 只在 `live + mock=false` 路徑下生效
- request-level `mode` 若明確指定，仍可覆蓋 backend 全域 preview 設定
- mock 啟動條件不應綁定 provider credential 是否存在
- 操作者可以透過不同啟動指令組合環境變數，快速切換 preview / mock / openai / gemma

provider 相關設定應維持各自獨立，例如：
- OpenAI: base URL、API key、default model
- Gemma: provider endpoint、credential、default model

## Analyze Flow

```text
builder analyze request
      │
      ├─ execution mode？
      │   ├─ preview_full
      │   │   └─ 直接回傳完整 preview response
      │   │      ├─ instructions
      │   │      ├─ user message text
      │   │      └─ attachments 摘要
      │   ├─ preview_prompt_body_only
      │   │   └─ 直接回傳 prompt body preview
      │   └─ live
      │       ├─ mock mode？
      │       │   ├─ 是 -> 直接回 mock analyze
      │       │   └─ 否
      │       └─ provider？
      │           ├─ openai
      │           │   ├─ attachments 存在？ -> upload Files API
      │           │   ├─ call Responses API
      │           │   └─ parse structured JSON
      │           └─ gemma
      │               ├─ 依 provider contract 適配附件 / content
      │               ├─ call live API
      │               └─ parse structured JSON
      └─ map provider error -> business error
```

## Consult Response Contract
AI 被要求回傳固定 JSON 結構：

```json
{
  "status": true,
  "statusAns": "",
  "response": "AI response text",
  "responseDetail": "internal detailed reasoning"
}
```

preview mode 下沿用同一個 business response contract，但內容語意不同：

```json
{
  "status": true,
  "statusAns": "PROMPT_PREVIEW",
  "response": "## [INSTRUCTIONS]\\n...\\n\\n## [USER_MESSAGE]\\n...",
  "responseDetail": ""
}
```

- `response` 應包含完整 preview 內容，而不是 AI 分析結果
- `responseDetail` 仍屬 business response contract 的一部分；preview / mock path 不依賴它承載主要內容
- preview 內容至少包含 instructions、user message text
- attachments 不需要真的 upload，但應列出本地摘要（fileName / contentType / size）

mock mode 下也沿用同一個 business response contract，但語意是 local/dev fallback output，而不是 live provider output。

## Preview Output Contract Variants

### `preview_full`

維持目前 contract：

```json
{
  "status": true,
  "statusAns": "PROMPT_PREVIEW",
  "response": "完整 prompt preview",
  "responseDetail": ""
}
```

### `preview_prompt_body_only`

目前 contract 仍沿用同一個 business response shape，但 `response` 的內容語意改為：

```json
{
  "status": true,
  "statusAns": "PROMPT_PREVIEW",
  "response": "builder 已組裝好的 prompt body",
  "responseDetail": ""
}
```

規則：
- `response` 只應包含主體 prompt 內容
- 不應包含：
  - `[INSTRUCTIONS]`
  - `[EXECUTION_RULES]`
  - `[RAW_USER_TEXT]`
  - `[PROMPT_BLOCK-*]`
  - `[USER_MESSAGE]`
  - JSON response contract 說明
- 第一版若走 astrology / profile prompt tuning，`response` 應優先對應 profile body render 後的主要內容

## Live Provider Contract

### OpenAI provider

- 保留目前 OpenAI Files API + Responses API 路徑
- `model` 由 OpenAI provider 自己消化
- OpenAI 需要的 request/response payload 與錯誤 mapping 由 provider 實作自己處理

### Gemma provider

- 新增獨立 provider 實作
- `model` 由 Gemma provider 自己消化
- 若 Gemma live API 與 OpenAI contract 不同，差異應限制在 provider 實作內，不應滲回 builder 或 gatekeeper
- Gemma 是否支援附件、支援方式為何，應由 provider contract 明確定義，不可強行套用 OpenAI Files API 假設
