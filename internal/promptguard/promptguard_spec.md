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
userText
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

目前 text scoring 已落成第一版 rule-based classifier：
- 明顯安全 -> `allow`
- 明顯高風險 -> `block`
- 灰區 -> `needs_llm`

第二層 LLM guard 仍只在 `needs_llm` 時才觸發。

## First-Layer Text Risk Classifier Domain
第一層 `text scoring` 的本質不是生成答案，而是對 `userText` 做風險分類。

應先把它理解成：

```text
userText
  -> normalization
  -> feature matching
  -> rule categories
  -> risk scoring
  -> decision routing
  -> explainability / match trace
```

這條線後續真正落地時，至少有 6 個核心概念：

1. `Normalization`
   - 先把文字整理成穩定可比對的形態
   - 例如大小寫統一、空白正規化、全半形處理、零寬字元過濾
2. `Feature Matching`
   - 從文字中抓出可疑特徵
   - 例如 keyword、phrase、regex、combo rule
3. `Rule Categories`
   - 把命中的規則歸類成風險型別
   - 例如 `override_attempt`、`prompt_exfiltration`、`role_spoofing`、`safety_bypass`
4. `Risk Scoring`
   - 把命中的 feature 轉成可累加的風險分數
5. `Decision Routing`
   - 依 score threshold 決定 `allow`、`block`、`needs_llm`
6. `Explainability / Match Trace`
   - 除了最終 decision 外，也要能追蹤命中的規則與分類

這表示：
- 第一層本質上是 text risk classifier，不是回答生成器
- 它的責任是把「明顯安全 / 明顯危險 / 灰區」先分流
- 灰區才升級到第二層 Gemma/local guard

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
- 接收單一 candidate text
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
- 不在第一版內實作完整 production-grade 規則覆蓋與調參系統
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
├─ ScoreText(userText)
└─ EvaluateWithLLM(command)
```

說明：
- `UseCase` 對外只暴露單一 `Evaluate`
- `Service` 內部分成三個方法
- 不因為有 text scoring 與 llm guard 兩條路，就拆成兩個 usecase
- 第二層 builder / aiclient orchestration 應封裝在 `promptguard service` 內，不回流到 `gatekeeper`
- 不另外拆新 module；第一層規則引擎仍屬於 `promptguard` 內部責任
- `promptguard command` 仍只帶一段 candidate text；current runtime 由 gatekeeper 依序把 `userText` 與 transport 直接帶入的 `intentText` 分別送入

`ScoreText` 的內部細部架構建議再拆成 pipeline：

```text
ScoreText(userText)
  -> normalize(userText)
  -> match(normalizedText)
  -> categorize(matches)
  -> score(matches, categories)
  -> route(score, matches, categories)
  -> build evaluation
```

理由：
- 不要一邊 match 一邊直接 return allow/block
- 要先收集訊號，再統一做 score 與 routing
- 這樣後續比較容易調 threshold、debug false positive、保留 explainability

建議檔案切法：

```text
internal/promptguard/
  usecase.go
  service.go
  model.go
  config.go
  text_normalizer.go
  feature_matcher.go
  rule_catalog.go
  score_engine.go
  decision_router.go
```

責任：
- `text_normalizer.go`
  - 正規化文字
- `feature_matcher.go`
  - 套規則並產生命中 feature
- `rule_catalog.go`
  - 集中管理 rule 資料
- `score_engine.go`
  - 計算 score 與 matched categories
- `decision_router.go`
  - 把 score 映射成 `allow / block / needs_llm`

建議 command 至少包含：
- `userText`
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

目的：
- 讓 risk scoring 可 debug
- 讓 false positive / false negative 可追查
- 讓 threshold 調整有依據

目前第一版已包含：
- `matchedRules`
- `matchedCategories`

建議另外保留一層內部分析模型，而不是只剩對外 `Evaluation`：

```text
TextAnalysis
├─ rawText
├─ normalizedText
├─ matchedRules
├─ matchedCategories
├─ score
├─ decision
└─ reason
```

說明：
- `TextAnalysis` 供內部 normalizer / matcher / scorer / router 串接
- `Evaluation` 仍是 module 對外統一結果
- 不要把所有內部 trace 都直接混進外部 contract

第一版 `model.go` 建議至少有以下概念型別：

```text
Decision
├─ allow
├─ block
└─ needs_llm

Source
├─ text_rule
└─ llm_guard

RuleCategory
├─ override_attempt
├─ prompt_exfiltration
├─ role_spoofing
└─ safety_bypass
```

說明：
- `Decision`
  - 作為整個 module 的最終 routing key，不應在程式中散落裸字串
- `Source`
  - 表示這次結果來自第一層 text rule，還是第二層 llm guard
- `RuleCategory`
  - 給 matcher、scorer、router、trace 共用
  - 第一版先收斂，不一開始把 category 拆得過細

建議的資料層次：

```text
Rule
  -> RuleMatch
     -> TextAnalysis
        -> Evaluation
```

意義：
- `Rule`
  - 靜態規則定義
- `RuleMatch`
  - 單次請求的動態命中證據
- `TextAnalysis`
  - 第一層 pipeline 的完整中間結果
- `Evaluation`
  - module 對外暴露的輕量決策結果

## Decision Rule

```text
promptguard evaluate
│
├─ Step 1: ScoreText(userText)
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
第一版 text scoring 已採 rule-based pipeline：

```text
ScoreText(userText)
  -> normalize(userText)
  -> match features
  -> map categories
  -> accumulate score
  -> route decision
  -> return evaluation + trace
```

目前行為：
- `normalized text` 先經過：
  - lowercase
  - trim
  - 多空白合併
  - 換行正規化
  - 全半形統一
  - 零寬字元過濾
- matcher 以 rule catalog 產生 `RuleMatch`
- score engine 直接累加 `RuleMatch.Weight`
- decision router 以 threshold 做：
  - `allow`
  - `block`
  - `needs_llm`

第一版 decision 方向：
- `score=0` -> `allow`
- `score>=8` -> `block`
- 其餘 -> `needs_llm`

第一版已能直接攔截明顯 prompt injection / override / exfiltration 類型文字，不再把所有輸入都升級到 LLM。

產品決策補充：
- `提示詞`
- `prompts`
- `promots`

這三個詞在目前專案上下文中，被視為極高風險詞彙。
第一版 rule catalog 會把它們直接視為高風險 `keyword`，命中後應直接進入 `block`，不再當成灰區 meta 詞處理。

rule 管理原則：
- rule 應盡量以資料結構集中管理，不要散成很多 `if strings.Contains(...)`
- rule 應自帶：
  - rule id
  - category
  - match type
  - pattern / phrase / combo
  - weight
- category 應在 rule 命中時就能確定，不要留到後面再硬猜

第一版 `Rule` 設計建議：

```text
Rule
├─ ID
├─ Category
├─ MatchType
├─ Weight
├─ Enabled
├─ Pattern
└─ Terms
```

欄位意圖：
- `ID`
  - 供 log、trace、測試斷言與後續 rule 維護使用
- `Category`
  - 直接表達這條 rule 屬於哪一種風險型別
- `MatchType`
  - 決定 matcher 如何解讀這條 rule
- `Weight`
  - 命中後累加到總分
- `Enabled`
  - 方便暫時停用某條 rule，不必修改 matcher 流程
- `Pattern`
  - 給 `keyword / phrase / regex` 使用
- `Terms`
  - 給 `combo` 使用
- 預處理欄位
  - 靜態 catalog 初始化後，應額外持有 pre-normalized pattern / terms 與 pre-compiled regex，避免 request hot path 重複處理靜態資料

第一版 `MatchType` 建議只保留 4 種：
- `keyword`
- `phrase`
- `regex`
- `combo`

設計原則：
- `Rule` 應該是資料，而不是 scattered branch logic
- 新增或調整 rule 時，應優先改 rule catalog，而不是去改 matcher 主流程
- 第一版先避免塞入過多 metadata，例如 `severity`、`confidence`、`description`
- 等第一版實戰後，再決定是否需要更多欄位

`RuleMatch` 設計建議：

```text
RuleMatch
├─ RuleID
├─ Category
├─ MatchType
├─ Weight
└─ Evidence
```

欄位意圖：
- `RuleID`
  - 表示這次實際命中了哪條 rule
- `Category`
  - 讓 scorer / router 不必再次回查 rule catalog
- `MatchType`
  - 保留這次命中的 feature 類型
- `Weight`
  - 保留這條命中對總分的實際貢獻
- `Evidence`
  - 表示這次真正抓到的文字證據，例如 phrase、regex 片段、combo 摘要

說明：
- `Rule` 是靜態定義
- `RuleMatch` 是單次請求中的動態證據
- explainability、debug、false positive / false negative 排查，都應以 `RuleMatch` 為核心

normalizer 第一版建議只做必要轉換：
- lowercase
- trim
- 多空白合併
- 換行正規化
- 全半形統一
- 去除零寬字元

matcher 第一版責任：
- 只負責把 normalized text 套 rule catalog
- 只負責回傳 `[]RuleMatch`
- 不在 matcher 中直接做 allow/block/needs_llm 決策
- 靜態 rule catalog 的 pattern / terms normalize 與 regex compile，應在 catalog 初始化時先做完，不應在每次 request 中重複執行
- 若 regex rule 在初始化時 compile 失敗，第一版應停用該 rule，而不是讓 request path 直接 panic
- `keyword` 與 `phrase` 在第一版目前都採 substring matching；兩者差異先只體現在 catalog 權重與命名意圖上，不在 matcher 行為上分流

score engine 第一版責任：
- 直接累加 `RuleMatch.Weight`
- 同步收集 matched categories
- 產生簡短 reason
- 第一版先不做複雜 category multiplier，後續再視實戰調整

decision router 第一版責任：
- 吃 score / matches / categories
- 回 `allow / block / needs_llm`
- 先採 threshold-based routing，不在第一版加入過多特例判斷

第一版 service 對外責任應收斂為：
- `ScoreText(userText)`
  - 僅負責第一層 text classifier
- `EvaluateWithLLM(command)`
  - 僅負責第二層 Gemma/local guard
- `Evaluate(command)`
  - 作為總控，統一路由 `allow / block / needs_llm`

第一版測試規劃：
- `text_normalizer` tests
- `feature_matcher` tests
- `score_engine` tests
- `decision_router` tests
- `ScoreText` integration-like tests

第一版實作順序已收斂為：
1. `model.go`
2. `rule_catalog.go`
3. `text_normalizer.go`
4. `feature_matcher.go`
5. `score_engine.go`
6. `decision_router.go`
7. `service.ScoreText()` 接上真正 pipeline
8. 補齊測試
9. 文件回寫同步

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
├─ candidateText
├─ builder identity
├─ app identity
├─ consult mode
└─ optional analysis type summary
```

第一版不應帶入：
- 另一段 profile text
- source prompts
- rag contents
- attachments
- 完整 main consult instructions
- astrology / mbti 業務分析內容

理由：
- prompt injection 的判斷目標是單一 candidate text
- current runtime 尚未在 transport 層建立 `intentText` 的可信來源邊界，所以 gatekeeper 會把 transport 直接帶入的 `intentText` 也當成 candidate text 送進 promptguard
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
- 對於模型偶發回的 markdown code fence 或 wrapper text，aiclient 應先做最小容錯清理；只有仍無法抽出第一段合法 JSON object 時，才視為真正的 guard json parse failure

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
- `promptguard` 一次只看單一 candidate text
- current runtime 中 gatekeeper 會分別把 `userText` 與 transport 直接帶入的 `intentText` 各自送入 promptguard
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
