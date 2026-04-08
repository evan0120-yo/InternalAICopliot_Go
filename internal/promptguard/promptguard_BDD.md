# PromptGuard BDD Spec

## Purpose
這份文件定義 `promptguard` module 第一版應滿足的行為規格。

這版重點是：
- 先把 promptguard 主 decision flow 接好
- 保留 text scoring 與 llm guard 兩層結構
- text scoring 暫時固定回 `needs_llm`
- llm guard 先切出 cloud / local 路由

## Actors
- caller：把 `raw user text` 交給 promptguard 做風險判定的上游 module
- promptguard usecase：對外暴露單一 `Evaluate` 入口
- promptguard service：執行 text scoring 與 llm guard routing

## Scenario Group: Module Layering
- Given `promptguard` module 對外提供風險判定能力
  When 設計 module layering
  Then 應採 `UseCase -> Service`
  And 不應因為有 text scoring 與 llm guard 兩條路就拆成兩個 usecase

- Given `promptguard` module 第一版
  When 對外暴露 public entry
  Then usecase 應只保留單一 `Evaluate(rawUserText)` 入口

## Scenario Group: Service Method Split
- Given `promptguard` service
  When 第一版設計內部方法
  Then service 應至少有：
  And `Evaluate(rawUserText)`
  And `ScoreText(rawUserText)`
  And `EvaluateWithLLM(rawUserText)`

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
- Given `Evaluate(rawUserText)` 被呼叫
  When promptguard 執行第一版主流程
  Then 應先執行 `ScoreText(rawUserText)`

- Given `ScoreText(rawUserText)` 回傳 `decision=block`
  When `Evaluate(rawUserText)` 繼續判定
  Then 應直接回 block
  And 不應呼叫 `EvaluateWithLLM(rawUserText)`

- Given `ScoreText(rawUserText)` 回傳 `decision=allow`
  When `Evaluate(rawUserText)` 繼續判定
  Then 應直接回 allow
  And 不應呼叫 `EvaluateWithLLM(rawUserText)`

- Given `ScoreText(rawUserText)` 回傳 `decision=needs_llm`
  When `Evaluate(rawUserText)` 繼續判定
  Then 應呼叫 `EvaluateWithLLM(rawUserText)`

## Scenario Group: LLM Guard Routing
- Given `EvaluateWithLLM(rawUserText)` 被呼叫
  And `INTERNAL_AI_COPILOT_PROMPTGUARD_MODE=cloud`
  When promptguard 執行第二層判定
  Then 應走 cloud gemma 路徑

- Given `EvaluateWithLLM(rawUserText)` 被呼叫
  And `INTERNAL_AI_COPILOT_PROMPTGUARD_MODE=local`
  When promptguard 執行第二層判定
  Then 應走 local gemma 路徑

- Given 第一版 LLM guard
  When cloud / local 路由已切出
  Then 這版只要求方法與切換點存在
  And 不要求這版同時完成最終 request body 與 parser 細節
  And cloud 路徑可先固定回 `decision=needs_llm`
  And local 路徑可先固定回 `decision=needs_llm`

## Scenario Group: Startup Configuration
- Given `promptguard` 需要與主分析模型設定解耦
  When backend 啟動
  Then `promptguard` 應使用自己的環境變數
  And 不應直接重用主分析 provider 的 mode 設定

- Given `INTERNAL_AI_COPILOT_PROMPTGUARD_MODE=cloud`
  When 啟動 promptguard
  Then 應視為 hosted gemma guard path

- Given `INTERNAL_AI_COPILOT_PROMPTGUARD_MODE=local`
  When 啟動 promptguard
  Then 應視為 local gemma guard path

- Given `INTERNAL_AI_COPILOT_PROMPTGUARD_MODE` 缺失或非法
  When 啟動 promptguard
  Then 第一版應 fallback 到 `cloud`

## Scenario Group: Boundary Rule
- Given `promptguard` 第一版
  When 執行風險判定
  Then 只應看 `raw user text`
  And 不應解析附件內容
  And 不應解析 builder 組裝後的完整 instructions
  And 不應理解 astrology / mbti / profile payload 業務內容

## Acceptance Notes
- 第一版重點是 decision flow 與 module boundary，不是完整規則品質
- text scoring 在第一版是 placeholder，目的是先把主流程接起來
- LLM guard 在第一版先切 cloud / local 路由，不強迫一次完成所有 provider 細節
- 這版文件已確認一個 usecase + 一個 service 的結構，不採兩個 usecase 分拆
