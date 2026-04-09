# AIClient BDD Spec

## Purpose
這份文件定義 aiclient module 目前應滿足的行為規格。內容以現有 code 與測試為基準。

## Actors
- builder consult usecase：將已組裝好的 instructions、user text 與附件交給 aiclient
- aiclient usecase：對 builder 暴露單一 analyze 入口
- promptguard service：在 text scoring 需要第二層 LLM 判定時，呼叫 aiclient 的 dedicated guard analyze

## Scenario Group: Analyze Mode Selection
- Given preview mode 開關為 true
  When `AnalyzeService.Analyze` 執行
  Then 不應呼叫外部 OpenAI API
  And 應直接回傳 `status=true`、`statusAns=PROMPT_PREVIEW`
  And `response` 應包含完整 AI request preview

- Given preview mode 開關為 true 且 `OpenAIAPIKey` 有值
  When `AnalyzeService.Analyze` 執行
  Then 仍應優先走 preview mode
  And 不應上傳附件到 Files API

- Given request 明確指定 `mode=live`
  When `AnalyzeService.Analyze` 執行
  Then 應覆蓋全域 preview 開關
  And 後續應依 `mock mode + provider` 決定 live path

## Scenario Group: Execution Mode And Provider Selection
- Given `INTERNAL_AI_COPILOT_AI_PROFILE=1`
  When `AnalyzeService.Analyze` 執行
  Then 不應呼叫外部 AI provider
  And 應直接回傳完整 prompt preview

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=2`
  When `AnalyzeService.Analyze` 執行
  Then 不應呼叫外部 AI provider
  And 應只回 builder 已組裝好的 prompt body

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=3`
  When `AnalyzeService.Analyze` 執行
  Then 應走 mock analyze
  And 不應呼叫外部 AI provider

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=4`
  When `AnalyzeService.Analyze` 執行
  Then 應走 OpenAI provider live path

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=5`
  When `AnalyzeService.Analyze` 執行
  Then 應走 Gemma provider live path

- Given request 明確指定 `mode=live`
  When backend 全域預設為 preview family
  Then 仍應覆蓋全域 preview 設定
  And 後續應依 `mock mode + provider` 決定 live path

- Given provider credential 缺值
  When `AnalyzeService.Analyze` 執行
  Then 不應自動把 credential 缺值視為 mock mode
  And mock 是否啟用應由顯式設定決定

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
```text
analyze
     │
     ├─ execution mode？
     │   ├─ preview family -> 直接回 preview
     │   └─ live
     │       ├─ mock mode -> 直接回 mock analyze
     │       └─ provider=openai
     ├─ normalize instructions / user text
     ├─ attachments 存在？
     │   ├─ 是 -> upload Files API
     │   │        ├─ image -> input_image / vision
     │   │        └─ file  -> input_file / user_data
     │   └─ 否
     ├─ call Responses API
     ├─ parse structured JSON
     └─ map OpenAI errors -> business errors
```

## Scenario Group: Gemma Provider Analyze
- Given analyze mode 為 `live`
  And `INTERNAL_AI_COPILOT_AI_PROFILE=5`
  When analyze 執行
  Then aiclient 應走 Gemma provider live path
  And builder / gatekeeper 不需要知道 Gemma request shape

- Given Gemma provider 與 OpenAI provider 的附件契約不同
  When analyze 執行
  Then 差異應封裝在 provider 實作內
  And 不應把 provider-specific 附件假設擴散到 builder

## Scenario Group: PromptGuard Analyze
```text
promptguard service
      │
      ├─ mode=cloud -> hosted Gemma
      └─ mode=local -> local Gemma
             │
             └─ dedicated guard JSON
```

- Given promptguard service 發起第二層 LLM guard
  And `INTERNAL_AI_COPILOT_AI_PROFILE=4`
  When aiclient 執行 promptguard analyze
  Then 應使用 promptguard 專用 cloud config
  And 應將 request 送到 hosted Gemma

- Given promptguard service 發起第二層 LLM guard
  And `INTERNAL_AI_COPILOT_AI_PROFILE=6`
  When aiclient 執行 promptguard analyze
  Then 應使用 promptguard 專用 local config
  And 應將 request 送到 local Gemma endpoint

- Given promptguard analyze 走 `cloud`
  When aiclient 組 request
  Then 應要求 API key
  And 缺值時應回 `PROMPTGUARD_GEMMA_API_KEY_MISSING`

- Given promptguard analyze 走 `local`
  When aiclient 組 request
  Then local route 可省略 API key
  And 缺少 `BaseURL` 時應回 `PROMPTGUARD_LOCAL_BASE_URL_MISSING`

- Given promptguard analyze 收到合法 guard JSON
  When aiclient 完成 parse
  Then 應回傳 dedicated guard result
  And `status=true` 應代表 allow
  And `status=false` 應代表 block

- Given promptguard analyze 收到 `status=false`
  When aiclient 完成 parse
  Then 不應把這類結果視為 provider error
  And 不應自行轉成 HTTP/business failure

- Given promptguard analyze 發生 transport failure、provider failure、JSON parse failure 或 contract mismatch
  When aiclient 處理回應
  Then 應回傳系統錯誤給 promptguard service
  And 不應自行合成 `status=false` 的 block result

## Scenario Group: Prompt Preview
- Given preview mode 開關為 true
  When analyze 執行
  Then `response` 應包含 instructions、user message text

- Given preview mode 開關為 true 且有附件
  When analyze 執行
  Then `response` 應包含附件摘要
  And 不應包含真實 OpenAI file id

- Given preview mode 開關為 true
  When builder 已將 structured profile data 組進 instructions
  Then preview 內容應原樣保留這些組裝結果
  And aiclient 不應額外理解 module 業務語意

- Given OpenAI 模式啟用
  When analyze 執行
  Then request payload 必須要求 JSON schema 格式，欄位固定為 `status`、`statusAns`、`response`、`responseDetail`

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
- preview mode 是 local/dev 觀察 prompt 的正式模式，不是臨時 debug print
- LinkChat profile-analysis 的 module 組合與理論資料，由 builder 負責組 prompt，aiclient 不應理解其業務語意
- mock mode 由顯式啟動設定控制，不再由 provider credential 缺值隱式觸發
- execution mode 與 live provider 分開決策，不混在同一個欄位
- promptguard 的第二層 LLM guard 使用獨立 env 與 dedicated guard JSON，不與主 consult analyze contract 混用

## Scenario Group: Preview Output Variants

- Given preview 輸出策略為 `preview_full`
  When analyze 執行
  Then 應維持目前完整 preview 行為
  And `response` 應包含完整 prompt preview
  And business response contract 仍保留 `responseDetail` 欄位

- Given preview 輸出策略為 `preview_prompt_body_only`
  When analyze 執行
  Then 不應呼叫 OpenAI
  And `response` 應只包含 builder 已組裝好的 prompt body
  And `response` 不應包含 `[INSTRUCTIONS]`
  And `response` 不應包含 `[EXECUTION_RULES]`
  And `response` 不應包含 `[RAW_USER_TEXT]`
  And `response` 不應包含 `[PROMPT_BLOCK-*]`
  And `response` 不應包含 `[USER_MESSAGE]`
  And `response` 不應包含 JSON response contract 說明文字
  And business response contract 仍保留 `responseDetail` 欄位

- Given preview 輸出策略為 `preview_prompt_body_only`
  When 這條線服務 astrology / profile prompt tuning
  Then 操作者應可直接複製 `response` 內容到外部 web GPT 做 prompt 調適
  And 不需要再自行從完整 preview 中手動裁切主體段落

- Given analyze mode 為 `live`
  When analyze 執行
  Then 應維持目前 OpenAI analyze 流程

## Code-Backed Tests
- `service_test.go`
- `service_preview_test.go`

## Open Questions
- 目前沒有獨立測試直接驗證 OpenAI HTTP payload 與上傳流程
- prompt injection 判斷仍是 keyword heuristic，未來是否要抽成可設定策略尚未定案
- Gemma provider 目前以 Gemini API `generateContent` + Files upload 實作；若官方 hosted Gemma contract 後續變動，需再同步核對
