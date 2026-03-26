# AIClient Module Spec

## Purpose
這份文件是 aiclient module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
AIClient 負責與 OpenAI Responses API 通訊。接收 builder usecase 組裝好的 prompt 和附件，呼叫 AI 取得結構化回應。

對 LinkChat profile-analysis 這條線來說，AIClient 看到的已經是 builder 組裝完成的 instructions 與 user text，不直接解析 `analysisPayloads`、`subjectProfile` 或理論值本身。

對應 Java：`com.citrus.internalaicopilot.aiclient`

## Layering In This Module

```text
UseCase -> Service
```

Repository 在第一版通常不是必要的；若未來需要持久化 audit/cache，再另行增加。

## Responsibilities
- 接收 instructions、user text、attachments
- 正規化 user text
- 上傳附件到 OpenAI Files API
- 分類附件為 image 或 file
- 呼叫 Responses API
- 解析結構化回應
- 映射 OpenAI 錯誤為 business error
- 將 module-specific profile data 視為 opaque prompt content，而不是在 aiclient 內重做業務判讀

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

## Analyze Flow

```text
builder analyze request
      │
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
  "response": "AI response text"
}
```
