# AIClient BDD Spec

## Purpose
這份文件定義 aiclient module 目前應滿足的行為規格。內容以現有 code 與測試為基準。

## Actors
- builder consult usecase：將已組裝好的 instructions、user text 與附件交給 aiclient
- aiclient usecase：對 builder 暴露單一 analyze 入口

## Scenario Group: Analyze Mode Selection
- Given `OpenAIAPIKey` 為空
  When `AnalyzeService.Analyze` 執行
  Then 應走 mock analyze 流程，不呼叫外部 OpenAI API

- Given `OpenAIAPIKey` 有值
  When `AnalyzeService.Analyze` 執行
  Then 應走 OpenAI Responses API 與 Files API 流程

## Scenario Group: Mock Analyze
- Given instructions 中的 `[RAW_USER_TEXT]` 看起來像 prompt injection
  When mock analyze 執行
  Then 應回傳 `status=false`、`statusAns=prompts有違法注入內容`、`response=取消回應`

- Given builderCode 可從 instructions 解析為 `qa-smoke-doc`
  When mock analyze 執行
  Then 應回傳固定的 QA 冒煙測試 markdown table 內容

- Given builderCode 不是 `qa-smoke-doc`
  When mock analyze 執行
  Then 應回傳一般的需求理解、功能拆解與風險摘要

- Given mock analyze 收到附件
  When builderCode 不是 `qa-smoke-doc`
  Then 回應內容應補上附件數量已納入分析上下文的說明

## Scenario Group: OpenAI Analyze
- Given OpenAI 模式啟用
  When analyze 執行
  Then request payload 必須要求 JSON schema 格式，欄位固定為 `status`、`statusAns`、`response`

- Given builder 已將 structured profile data 組進 instructions
  When analyze 執行
  Then aiclient 應將這些內容視為 opaque prompt text，不在 aiclient 內重做 module 判讀

- Given有附件
  When analyze 執行
  Then 每個附件應先上傳 Files API，再依檔名決定作為 `input_image` 或 `input_file`

- Given圖片副檔名
  When upload purpose 決定時
  Then purpose 應為 `vision`

- Given非圖片附件
  When upload purpose 決定時
  Then purpose 應為 `user_data`

- Given OpenAI Responses API 或 Files API 回傳 4xx/5xx
  When aiclient 處理外部回應
  Then 應回傳 `OPENAI_ANALYSIS_FAILED`、`ATTACHMENT_UPLOAD_FAILED` 或 `ATTACHMENT_UPLOAD_REJECTED`

- Given OpenAI 回傳空 output 或不符合預期 JSON
  When aiclient 解析 structured output
  Then 應回傳 `OPENAI_EMPTY_OUTPUT` 或 `OPENAI_ANALYSIS_FAILED`

## Acceptance Notes
- aiclient 對外行為結果固定為 `infra.ConsultBusinessResponse`
- mock mode 是目前本地開發與測試的重要 fallback，不是暫時性 stub
- LinkChat profile-analysis 的 module 組合與理論資料，由 builder 負責組 prompt，aiclient 不應理解其業務語意

## Code-Backed Tests
- `service_test.go`
- `service_preview_test.go`

## Open Questions
- 目前沒有獨立測試直接驗證 OpenAI HTTP payload 與上傳流程
- prompt injection 判斷仍是 keyword heuristic，未來是否要抽成可設定策略尚未定案
