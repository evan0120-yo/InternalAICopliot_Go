# PromptGuard BDD Spec

## Purpose
這份文件定義 `promptguard` module 第一版應滿足的行為規格。

這版重點是：
- 先把 promptguard 主 decision flow 接好
- 保留 text scoring 與 llm guard 兩層結構
- text scoring 暫時固定回 `needs_llm`
- llm guard 先切出 cloud / local 路由
- 和星座主流程的串接點先定在 `gatekeeper usecase`

## Actors
- caller：把 `raw user text` 交給 promptguard 做風險判定的上游 module
- promptguard usecase：對外暴露單一 `Evaluate` 入口
- promptguard service：執行 text scoring 與 llm guard routing

## Scenario Group: Module Layering
- Given `promptguard` module 對外提供風險判定能力
  When 設計 module layering
  Then 應採 `UseCase -> Service`
  And 不應因為有 text scoring 與 llm guard 兩條路就拆成兩個 usecase

- Given `promptguard` 要接到星座主流程
  When 決定主流程串接位置
  Then 應由 `gatekeeper usecase` 呼叫 `promptguard usecase`
  And 不應直接把 promptguard 邏輯塞進 `gatekeeper service`

- Given `promptguard` module 第一版
  When 對外暴露 public entry
  Then usecase 應只保留單一 `Evaluate(command)` 入口

## Scenario Group: Service Method Split
- Given `promptguard` service
  When 第一版設計內部方法
  Then service 應至少有：
  And `Evaluate(command)`
  And `ScoreText(rawUserText)`
  And `EvaluateWithLLM(command)`

- Given 第二層 LLM guard 需要 builder 與 AI provider
  When promptguard 進入 `needs_llm` path
  Then 應由 `promptguard service` 自己向下呼叫 `builder` 與 `aiclient`
  And 不應把這段 orchestration 回流給 `gatekeeper`

## Scenario Group: Evaluation Contract
- Given `promptguard` 回傳風險判定結果
  When 任一方法完成 evaluation
  Then 不應只回裸分數
  And 應至少回傳：
  And `decision`
  And `score`
  And `reason`
  And `source`

- Given `decision`
  When 表示 promptguard 的判定結果
  Then 應限制為：
  And `allow`
  And `block`
  And `needs_llm`

- Given `source`
  When 表示判定來源
  Then 應限制為：
  And `text_rule`
  And `llm_guard`

## Scenario Group: First-Version Text Scoring Placeholder
- Given 第一版 `ScoreText(rawUserText)`
  When 尚未落地真正的 keyword / regex / scoring rule
  Then 不應自行猜測 allow 或 block
  And 應固定回傳 `decision=needs_llm`
  And `source` 應為 `text_rule`
  And `reason` 應為 placeholder reason
  And `score` 應為固定 placeholder 分數 `50`

## Scenario Group: Main Decision Flow
- Given `Evaluate(command)` 被呼叫
  When promptguard 執行第一版主流程
  Then 應先執行 `ScoreText(command.RawUserText)`

- Given `ScoreText(rawUserText)` 回傳 `decision=block`
  When `Evaluate(command)` 繼續判定
  Then 應直接回 block
  And 不應呼叫 `EvaluateWithLLM(command)`

- Given `ScoreText(rawUserText)` 回傳 `decision=allow`
  When `Evaluate(command)` 繼續判定
  Then 應直接回 allow
  And 不應呼叫 `EvaluateWithLLM(command)`

- Given `ScoreText(rawUserText)` 回傳 `decision=needs_llm`
  When `Evaluate(command)` 繼續判定
  Then 應呼叫 `EvaluateWithLLM(command)`

## Scenario Group: Main-Flow Integration
- Given 星座主流程第一版要接 promptguard
  When `ProfileConsult` 或 `PublicProfileConsult` request 通過原本 gatekeeper 驗證
  Then `gatekeeper usecase` 應先呼叫 `promptguard usecase`
  And guard 通過後才可進 `builderConsult.Consult`

- Given `promptguard` 回傳 block
  When `gatekeeper usecase` 接到 guard result
  Then 應直接中止主流程
  And 不應再進 `builderConsult.Consult`

- Given `promptguard` 回傳 allow
  When `gatekeeper usecase` 接到 guard result
  Then 應繼續原本主流程

- Given 第一期 rollout scope
  When 實作 promptguard 串接
  Then 應先只接星座 profile 流程
  And 不要求 generic consult 這版一起接上

## Scenario Group: LLM Guard Routing
- Given `EvaluateWithLLM(command)` 被呼叫
  And `INTERNAL_AI_COPILOT_AI_PROFILE=4`
  When promptguard 執行第二層判定
  Then 應走 cloud gemma 路徑

- Given `EvaluateWithLLM(command)` 被呼叫
  And `INTERNAL_AI_COPILOT_AI_PROFILE=6`
  When promptguard 執行第二層判定
  Then 應走 local gemma 路徑

- Given 第二層 LLM guard 被觸發
  When promptguard 執行這段 path
  Then 應先由 `builder` 組 dedicated guard prompt
  And 再由 `aiclient` 依 `AI_PROFILE` 切 cloud/local model

- Given promptguard service 沒有 wiring builder assembler 或對應 llm route
  When `EvaluateWithLLM(command)` 被呼叫
  Then 應回 placeholder `decision=needs_llm`
  And cloud 應回 `LLM_GUARD_CLOUD_PLACEHOLDER`
  And local 應回 `LLM_GUARD_LOCAL_PLACEHOLDER`

- Given promptguard service 已 wiring builder assembler 與 llm route
  When `EvaluateWithLLM(command)` 被呼叫
  Then 應先組 dedicated guard prompt
  And 應呼叫對應的 cloud/local llm route
  And `status=true` 應映射為 `decision=allow`
  And `status=false` 應映射為 `decision=block`

## Scenario Group: Guard Prompt Boundary
- Given promptguard 第二層要向下組 prompt
  When builder 參與 guard path
  Then 應組 dedicated guard prompt
  And 該 prompt 應以 `raw user text` 為核心
  And 可帶 minimal builder/app/context metadata

- Given promptguard 第二層 guard prompt
  When 決定是否要帶 source 與 rag
  Then 第一版不應帶完整 source prompts
  And 第一版不應帶 rag contents
  And 第一版不應帶附件內容
  And 第一版不應重用 main consult 的整份 instructions

## Scenario Group: Guard JSON Contract
- Given promptguard 第二層 LLM guard
  When 要求 model 回傳結果
  Then 應要求回 JSON
  And 應至少包含 `status`
  And `status=true` 代表 allow
  And `status=false` 代表 block

- Given promptguard 第二層 LLM guard 的 JSON 無法解析
  When promptguard 完成 provider 呼叫
  Then 應視為 system error
  And 不應把它當成正常 block

## Scenario Group: Block Handling
- Given promptguard 判定 `status=false`
  When gatekeeper 接到 guard result
  Then 應回 blocked business response
  And 不應把這個 block 當成 HTTP 4xx validation error

## Scenario Group: Startup Configuration
- Given `promptguard` 需要與主分析模型設定解耦
  When backend 啟動
  Then `promptguard` 的主要啟動方式應跟主分析共用 `AI_PROFILE`
  And 不應要求操作者日常手設一整組 `PROMPTGUARD_*` env

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=4`
  When 啟動 promptguard
  Then 應視為 hosted gemma guard path

- Given `INTERNAL_AI_COPILOT_AI_PROFILE=6`
  When 啟動 promptguard
  Then 應視為 local gemma guard path

- Given `INTERNAL_AI_COPILOT_AI_PROFILE` 缺失或非法
  When 啟動 promptguard
  Then 應回退讀舊的 `PROMPTGUARD_*` 與主 Gemma 相容 env

- Given `AI_PROFILE` 已合法設定
  When 啟動 promptguard
  Then `AI_PROFILE` 應優先決定 cloud/local、model 與 base URL
  And `GEMINI_API_KEY` 應作為 cloud gemma 的主要 credential

## Scenario Group: Boundary Rule
- Given `promptguard` 第一版
  When 執行風險判定
  Then 只應看 `raw user text`
  And 不應解析附件內容
  And 不應解析 builder 組裝後的完整 instructions
  And 不應理解 astrology / mbti / profile payload 業務內容
  And 第二層 builder 只應組 dedicated guard prompt
  And 第一版不應載 source / rag

## Acceptance Notes
- 第一版重點是 decision flow 與 module boundary，不是完整規則品質
- text scoring 在第一版是 placeholder，目的是先把主流程接起來
- LLM guard 在 current wiring 下已會經過 builder+aiclient，未 wiring 時仍保留 placeholder fallback
- 這版文件已確認一個 usecase + 一個 service 的結構，不採兩個 usecase 分拆
