# AIClient Module Spec

## Purpose
這份文件是 aiclient module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
AIClient 負責與 OpenAI Responses API 通訊。接收 builder usecase 組裝好的 prompt 和附件，呼叫 AI 取得結構化回應。

對 LinkChat profile-analysis 這條線來說，AIClient 看到的已經是 builder 組裝完成的 instructions 與 user text，不直接解析 `analysisPayloads`、`subjectProfile` 或理論值本身。

另外保留一個 local/dev 用的 preview mode family：
- 不呼叫 OpenAI
- 由 backend 決定要回完整 preview 還是只回 prompt body
- 用途是讓前端直接檢查 prompt / user message / 附件摘要長什麼樣，或快速複製主體 prompt 去外部 web GPT 調適

## Current Preview Output Variants

目前 aiclient 已支援三種輸出 mode：

```text
preview_full
  -> 現在這種完整 preview

preview_prompt_body_only
  -> 只回 builder 已組裝好的 prompt body
  -> 不呼叫 OpenAI
  -> 不模擬 AI final answer

live
  -> 真正呼叫 OpenAI
```

其中 `preview_prompt_body_only` 的定位要非常明確：

- 它不是 `response only` 的 AI 結果模擬
- 它不是把完整 preview 交給前端後，再由前端自行裁字串
- 它是 backend 直接選擇只輸出 prompt body 的 preview 變體
- 目的主要是讓操作者快速 copy 這段組裝後的主體 prompt，到外部 web GPT 手動調 prompt

對應 Java：`com.citrus.internalaicopilot.aiclient`

## Layering In This Module

```text
UseCase -> Service
```

Repository 在第一版通常不是必要的；若未來需要持久化 audit/cache，再另行增加。

## Responsibilities
- 接收 instructions、user text、attachments
- 在 preview mode 下直接回傳完整 AI request preview
- 正規化 user text
- 上傳附件到 OpenAI Files API
- 分類附件為 image 或 file
- 呼叫 Responses API
- 解析結構化回應
- 映射 OpenAI 錯誤為 business error
- 將 module-specific profile data 視為 opaque prompt content，而不是在 aiclient 內重做業務判讀

- 若預覽策略為 `preview_prompt_body_only`，aiclient 應只輸出 builder 已組裝好的 prompt body
- aiclient 不應在這個 mode 內合成假的 AI 結果

## Layer Responsibilities

### UseCase
- 作為對 builder 暴露的分析入口
- 負責分析用例的 orchestration

### Service
- OpenAI request/response 細節
- temp file / upload / parse 邏輯
- error mapping

## Boundary Rule
- aiclient 不應決定哪些 modules 參與分析
- aiclient 不應理解 LinkChat 的星座、MBTI 或其他理論欄位
- aiclient 只負責把 builder 已組好的內容送到模型
- preview mode 也只回 builder 已組好的內容，不在 aiclient 內重做 prompt 組裝
- `preview_prompt_body_only` 仍遵守同一條邊界：aiclient 只輸出 builder 已組好的 body，不自己重新拼 profile 語意

## Analyze Flow

```text
builder analyze request
      │
      ├─ preview mode 開啟？
      │   ├─ 是 -> 直接回傳 preview response
      │   │        ├─ instructions
      │   │        ├─ user message text
      │   │        └─ attachments 摘要
      │   └─ 否
      ├─ instructions / user text 正規化
      ├─ attachments 存在？
      │   ├─ 是 -> 上傳 Files API
      │   └─ 否
      ├─ call Responses API
      ├─ parse structured JSON
      └─ map external error -> business error
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

## Planned Preview Output Contract Variants

### `preview_full`

維持目前 contract：

```json
{
  "status": true,
  "statusAns": "PROMPT_PREVIEW",
  "response": "完整 prompt preview"
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
