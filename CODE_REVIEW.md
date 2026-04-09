# Internal AI Copilot — Go Backend Code Review

---

# BLOCK 1: AI 對產品的想像

這個 Go backend 現在看起來像一個「可設定的 AI 諮詢引擎」。
真正的產品核心不是某個固定 prompt，而是 Builder。每個 Builder 代表一種諮詢任務，管理端維護 source / rag / template，前台或外部系統只要指定 builderId，就能走同一條 consult 引擎。

它的主要使用者有四種：
- 內部前台使用者：查 builders、送出一般 consult、拿文字或檔案結果。
- 外部整合系統：透過 `X-App-Id` 或 gRPC `appId` 走 app 授權。
- LinkChat：走 `ProfileConsult`，把結構化 profile 交給 Internal 組 prompt。
- 管理員：維護 builder graph 與 template library。

它目前比較像公司內部的小型 AI 平台，不是高流量 SaaS。
程式有很多「先全撈再 Go 層過濾」的做法，代表它預設的資料量不大。現在 seed data 只有 3 個 builders、1 個外部 app、1 組 app prompt config，這個假設很明顯。

從 code 看得出的刻意選擇有幾個：
- HTTP 和 gRPC 共用同一組 gatekeeper / builder / aiclient / output 邊界，不分兩套業務流程。
- generic consult 和 profile consult 被明確拆成兩個 mode，不讓 builder 自己猜。
- AI 執行邏輯已拆成 `preview / mock / live`，`live` 再切 `openai / gemma` provider。
- repo 內另外長出一個獨立的 `promptguard` module，而且現在已經接進 `linkchat-astrology` 的 profile 主流程；generic consult 仍不走這條 guard。
- LinkChat 的 prompt 差異不塞在 transport，而是塞在 builder 的 app-aware strategy。
- local 開發刻意保留 Firestore emulator + reset/seed on start。

它目前不是：
- 不是多輪聊天系統，沒有 session memory。
- 不是向量搜尋型 RAG，現在的 rag 幾乎都是靜態 `full_context` 補充內容。
- 不是完整的後台權限系統，admin routes 目前沒有另外做 auth。
- 不是一個已經 fully generic 的 profile 平台，現在真正有 seed graph 落地的是 LinkChat astrology 這條線。

---

# BLOCK 2: 讀者模式

## A. 系統啟動後，骨架怎麼接起來

這個服務啟動後會同時開 HTTP 和 gRPC，兩邊都接到同一套 use case。

```text
main 啟動
   │
   ├─ 讀 env config
   ├─ 建 Firestore store
   ├─ 組 rag / aiclient / output / builder / gatekeeper
   ├─ 註冊 HTTP routes
   ├─ 註冊 gRPC IntegrationService
   ├─ 開 HTTP server
   └─ 開 gRPC server
```

> 注意：CORS 只做精確 origin 比對，不是 wildcard。

> 注意：HTTP 和 gRPC 都有 panic / graceful shutdown / timeout，但 admin API 沒有額外 auth gate。

## B. 查 Builder 列表

前台或外部 app 打進來後，第一個常見動作是查「有哪些 builder 可以用」。

```text
ListBuilders
   │
   ├─ public
   │  └─ 撈全部 builders -> 過濾 active=true
   │
   └─ external
      ├─ 先驗 appId 存在且 active
      └─ 再把 active builders 用 allowedBuilderIds 白名單過濾
```

這裡沒有分頁，也沒有 Firestore where active=true 的 query。
目前就是先撈全部，再用 Go 層做過濾與排序。

> 注意：回傳排序基本上是 builderId 升序。

## C. 一般 Consult

一般 consult 是這個系統最核心的共享流程。
HTTP public、HTTP external、gRPC generic consult，最後都會走到同一個 builder consult use case。

```text
一般 consult 進來
   │
   ├─ 解析 transport
   │  ├─ HTTP: multipart/form-data
   │  └─ gRPC: protobuf attachments
   │
   ├─ gatekeeper 驗證
   │  ├─ client IP 要有
   │  ├─ builderId 要存在且 active
   │  ├─ outputFormat 若有值，只能 markdown / xlsx
   │  └─ 附件要通過數量 / 大小 / 副檔名限制
   │
   ├─ external caller？
   │  └─ 是的話再驗 appId 與 builder 白名單
   │
   └─ 進 builder consult 引擎
      │
      ├─ 併發載入 builder + sources
      ├─ 需要 rag 的 source 再併發補 rag
      ├─ assemble prompt
      ├─ aiclient 決定 preview / mock / live
      ├─ output 決定要不要產檔
      └─ 回前端或 gRPC caller
```

這條線的 prompt 組裝重點是：
- 真正的使用者原文不放在 AI `user` message，而是塞在 instructions 裡的 `[RAW_USER_TEXT]`。
- AI `user` message 目前是固定一句話，作用比較像「請依 instructions 執行」。
- source 會照 orderNo 依序組成 `[PROMPT_BLOCK-*]`。
- rag 會直接接在對應 source 後面。
- 若某個 rag 是 overridable，使用者 text 會覆蓋或插入 `{{userText}}`。

> 注意：public `POST /api/consult` 可以帶 `appId`，但這裡只是 pass-through hint，不做 external auth。

> 注意：generic consult 目前沒有 structured profile，所以這個 `appId` 通常不會造成可見差異。

## D. Profile Consult

Profile consult 是現在和 LinkChat 最相關的那條線。
這條線跟一般 consult 最大的差別，不是 transport，而是 builder 會先把 structured profile 轉成 app-aware prompt block，再決定哪些 source 參與。

### 你可以從哪裡進來

```text
Profile consult
   │
   ├─ HTTP /api/profile-consult
   │  ├─ JSON body
   │  ├─ appId 只當 prompt-strategy hint
   │  └─ 可用 request.mode 覆蓋 backend 預設 AI mode
   │
   └─ gRPC ProfileConsult
      ├─ appId 空 -> 走 public profile path
      └─ appId 有值 -> 走 external app auth path
```

### 它比一般 consult 多做的事

```text
profile request 通過 transport parse
   │
   ├─ subjectProfile 正規化
   │  ├─ subjectId 空 + analysisPayloads 空 -> 視為沒傳
   │  ├─ subjectId 空但 analysisPayloads 有值 -> 擋回
   │  ├─ analysisType -> trim / lowercase / regex 驗證
   │  ├─ analysisType 不可重複
   │  ├─ theoryVersion 若有傳不可空白
   │  └─ weighted payload envelope 先驗 shape
   │
   ├─ text 和 normalized subjectProfile 不能同時為空
   │
   ├─ 第一版 promptguard 條件命中？
   │  ├─ builderCode=linkchat-astrology 且 text 非空 -> 先做 guard
   │  └─ 否 -> 直接進 builder profile mode
   │
   └─ 進 builder profile mode
      │
      ├─ 先依 strategy 過濾 sources
      │  ├─ default -> 用 analysisType 當 tags
      │  └─ linkchat -> renderer 產 tags
      │
      ├─ 再 build [SUBJECT_PROFILE]
      │  ├─ default -> flatten payload 成 deterministic lines
      │  └─ linkchat -> 用 source graph 組語意片段
      │
      └─ 後面才接一般 consult 的 AI / output 流程
```

### default strategy 和 linkchat strategy 的差別

```text
default
   ├─ 不讀 appPromptConfig 時，預設走這條
   ├─ 只把 analysis payload flatten 成文字
   └─ source filtering 只看 analysisType 是否對上 moduleKey

linkchat
   ├─ 要先在 Firestore 找到 appPromptConfig(appId=linkchat)
   ├─ source filtering 先走 analysis renderer
   ├─ astrology:
   │  ├─ slot key 對 primary source.matchKey
   │  ├─ canonical value 對 fragment source.matchKey
   │  └─ fragment 再展開 sourceIds child graph
   └─ mbti:
      └─ 目前只做 raw payload flatten，不做 astrology 那種 graph lookup
```

> 注意：`theoryVersion` 現在只是 metadata，不參與 source lookup。

> 注意：LinkChat strategy 有 process-local cache，改 Firestore 的 appPromptConfig 後，不重啟不保證會立即吃到新設定。

> 注意：code 有 `mbti` renderer，但目前 seed builder 只放了 astrology source graph；如果沒有對應 `mbti` sources，profile source filtering 會直接變成空集合而失敗。

> 注意：`promptguard` 第一版只掛在 `linkchat-astrology` 且 `text` 非空的 profile request。其他 builders、generic consult、純 structured profile request 目前都直接跳過這層。

## E. AI 執行層現在怎麼切

AI 這層現在已經不是單純的 OpenAI-only service 了。
目前是先判 execution mode，再決定 live provider。

```text
Analyze
   │
   ├─ resolve mode
   │  ├─ request.mode 有值 -> 優先
   │  ├─ 否則看 AIDefaultMode
   │  ├─ 若 AIDefaultMode 空且 AIPreviewMode=true -> preview_full
   │  └─ 否則 -> live
   │
   ├─ preview_full
   │  └─ 回完整 preview
   │
   ├─ preview_prompt_body_only
   │  └─ 只回 builder 組好的主體 prompt body
   │
   └─ live
      ├─ AIMockMode=true -> mock analyze
      └─ AIMockMode=false -> provider
         ├─ openai
         └─ gemma
```

### 各 mode 的語意

- `preview_full`：回完整 instructions、固定 user message、附件摘要。
- `preview_prompt_body_only`：回 builder 提供的 `PromptBodyPreview`。
- `mock`：不打外部 AI，回內建假資料。
- `live/openai`：OpenAI Files API + Responses API。
- `live/gemma`：Google Generative Language API `generateContent` + resumable file upload。

> 注意：`preview_prompt_body_only` 現在實際上主要服務 profile prompt tuning。因為 builder 目前只會從 profile block 產生 `PromptBodyPreview`，一般 consult 多半會是空字串。

> 注意：request-level mode override 目前只有 HTTP `/api/profile-consult` 真的接上。一般 consult 和 gRPC generic consult 沒有這條 override。

> 注意：mock analyze 會想從 instructions 裡抓 `builderCode=...` 決定特殊模板，但目前正式 prompt 組裝沒有把這個 marker 放進去，所以 runtime 通常只會走 general mock fallback。

### PromptGuard 目前的位置

repo 裡現在已經有一個獨立的 `promptguard` module，而且第一版已經接進 LinkChat 星座 profile 主流程。

```text
ProfileConsult / PublicProfileConsult
   │
   ├─ gatekeeper validate
   ├─ builderCode=linkchat-astrology 且 text 非空？
   │  ├─ 否 -> 直接 builder consult
   │  └─ 是
   │     └─ promptguard.Evaluate
   │        ├─ ScoreText(rawUserText)
   │        │  ├─ no match -> allow
   │        │  ├─ high-risk -> block
   │        │  └─ gray area -> needs_llm
   │        └─ EvaluateWithLLM(command)
   │           ├─ builder.AssemblePromptGuard
   │           └─ aiclient.AnalyzeGuard
   │              ├─ cloud -> hosted Gemma
   │              └─ local -> local endpoint
   │
   ├─ guard allow -> builder consult
   ├─ guard block -> 正常 business response
   └─ guard failure -> system error
```

它現在不只是切 module 邊界，而是真的有 runtime 作用：
- 有自己的 usecase / service
- 有自己的 env
- 有 `allow / block / needs_llm` decision contract
- `app.New()` 會把它接到 gatekeeper / builder / aiclient
- 第二層 guard 已經會真的打 cloud/local Gemma analyze path
- 第一層不再只是 placeholder；現在會先做文字正規化、規則命中、分數累加，再依風險分數決定放行、攔截或升級到 Gemma
- 第一層結果會附帶 `matchedRules` / `matchedCategories` trace，方便之後調 rule 與排查誤殺

> 注意：`ScoreText` 現在已是第一版 rule-based classifier，只有灰區輸入才會升級到第二層 LLM guard。

> 注意：第二層 guard 的 builder path 是 dedicated prompt assembly，不讀 `source / rag / attachments / [SUBJECT_PROFILE]` 主分析內容。

> 注意：若 `promptguard` 專用 env 沒設，現在會優先讀 dedicated env；缺值時再回退讀主 Gemma 相容 env，避免 local/dev 需要維護兩套一樣的 key。

## F. Output 與檔案回傳

AI 回來之後，不一定會產檔。
真正會不會產 file，要再過一次 output policy。

```text
output render
   │
   ├─ preview response？ -> 不產檔
   ├─ status=false？ -> 不產檔
   ├─ builder.IncludeFile=false？ -> 不產檔
   │
   └─ 其餘才選格式
      ├─ request outputFormat 有值 -> 用 request
      └─ 否則 -> 用 builder.DefaultOutputFormat
         ├─ markdown
         └─ xlsx
```

目前 seed 只有 `qa-smoke-doc` 這個 builder 會產檔，預設格式是 xlsx。

> 注意：gRPC generic `Consult` 會把 file base64 解回 raw bytes 再回傳。

> 注意：gRPC `ProfileConsult` 只回 `status / statusAns / response`，就算 backend 內部真的有 file，也不會帶出去。

## G. Admin Graph

Admin graph 是管理某個 builder prompt 結構的地方。
現在可以讀完整 graph，也可以整包覆寫。

```text
LoadGraph
   └─ builder -> sources -> each source rags -> 回完整 graph

SaveGraph
   │
   ├─ strict JSON decode
   ├─ builder 欄位做 partial merge
   ├─ sources 正規化
   │  ├─ systemBlock=true 的 request source 直接忽略
   │  ├─ orderNo 重新 canonical 化
   │  ├─ moduleKey / sourceType / sourceIds / tags 正規化
   │  └─ rag 只接受 full_context
   │
   └─ Firestore transaction
      ├─ 保留 systemBlock 舊 source
      ├─ 刪掉既有非 system sources + rags
      ├─ 寫入新 sources + rags
      └─ 更新 counters
```

> 注意：這不是 diff update，而是「非 systemBlock 的整批替換」。

> 注意：request 若沒有 `sources`，會 fallback 嘗試讀舊版 `aiagent[].source` 形狀。

> 注意：read path 會把 rag 的 `retrievalMode` 統一輸出成 `full_context`。

## H. Admin Templates

Templates 是可重用的 source/rag 公版庫。
它跟 builder graph 分開存，但刪除 template 時，會去清 builder source 上的 copied-from metadata。

```text
List templates
   ├─ by builder -> 只看 active template
   │              groupKey 不符就過濾掉
   └─ all -> 全部列出

Create / Update template
   ├─ 驗 templateKey / name / orderNo / rag
   ├─ SaveTemplate 交易內重寫 template 與 templateRags
   └─ ReorderTemplates 再把整體 orderNo 排順

Delete template
   ├─ 刪 templateRags
   ├─ 刪 template
   ├─ 走訪所有 builders / sources
   │  清 copiedFromTemplate* 欄位
   └─ 再把其他 templates 重排
```

> 注意：template delete 目前不是 transaction，一半成功一半失敗時，可能留下部分清理過、部分沒清的狀態。

## I. 目前資料基線與已知限制

現在的 seed data 已經把系統用途講得很清楚：

```text
外部 app
└─ linkchat
   └─ allowedBuilderIds = [1, 2, 3]

builders
├─ 1 pm-estimate
├─ 2 qa-smoke-doc
└─ 3 linkchat-astrology

app prompt config
└─ linkchat -> strategy=linkchat

templates
└─ 4 個基礎 template
```

這份 seed data 也透露出目前的真實成熟度：
- PM builder 與 QA builder 是比較完整的一般 consult 路線。
- LinkChat 這條線的主體是 astrology composable source graph。
- `mbti` 雖然 code 先留了 renderer，但 seed graph 還沒把它補成可直接用的 builder data。

目前最值得讀者記住的限制有這些：
- admin routes 沒有 auth。
- public `/api/profile-consult` 與 public `/api/consult appId` 都偏向 local/dev 測試入口，code 沒有限制只能在 dev 用。
- app prompt config cache 沒有 TTL / invalidation。
- composable source graph 沒做 cycle guard。
- mock 的 builder-specific 分支目前大多數情況不會命中。

### 目前請求狀態流轉

```text
request 進來
   -> transport parse
   -> gatekeeper validate
   -> builder load
   -> optional profile source filter
   -> rag resolve
   -> prompt assemble
   -> aiclient preview/mock/live
   -> output render
   -> HTTP / gRPC mapping
```

### 目前持久化狀態最有意義的是這些

```text
Builder.Active
   true  -> 可出現在 builder list，也可被 consult
   false -> 仍存在 DB，但 consult 會被擋

Template.Active
   true  -> ListTemplatesByBuilder 會出現
   false -> 仍存在 DB，但 builder-specific list 會過濾掉

Source.SystemBlock
   true  -> admin SaveGraph 不會覆寫它
   false -> 屬於 graph replace 範圍
```

---

# BLOCK 3: 技術補充

## A. 系統啟動與 wiring

關鍵檔案：
- cmd/api/main.go (line 20)
- internal/app/app.go (line 27)
- internal/infra/config.go (line 37)
- internal/infra/store.go (line 89)

### 啟動順序

```text
main
  -> infra.LoadConfigFromEnv
  -> app.New
     -> infra.NewStoreWithOptions
     -> rag.NewResolveService / UseCase
     -> aiclient.NewAnalyzeService / UseCase
  -> output.NewRenderService / UseCase
  -> builder.NewQueryService
  -> builder.NewAssembleService
  -> builder.NewGraphService / UseCase
  -> builder.NewTemplateService / UseCase
  -> builder.NewConsultUseCase(cfg.ResolvedAIModel())
  -> gatekeeper.NewGuardService
  -> promptguard.LoadConfigFromEnv
  -> promptguard.NewService
     -> builder.AssemblePromptGuard closure
     -> aiUseCase.AnalyzeGuard cloud/local closures
  -> promptguard.NewEvaluateUseCase
  -> gatekeeper.NewUseCase(promptguard injected)
  -> gatekeeper.NewHandler
  -> builder.NewAdminHandler
  -> HTTP server
  -> gRPC server
```

### 目前重要 env

| 設定 | 來源 | 預設 |
| --- | --- | --- |
| HTTP 位址 | `INTERNAL_AI_COPILOT_ADDR` | `:8082` |
| gRPC 位址 | `INTERNAL_AI_COPILOT_GRPC_ADDR` | `:9091` |
| Firestore project | `INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID` | `dailo-467502` |
| Firestore emulator | `INTERNAL_AI_COPILOT_FIRESTORE_EMULATOR_HOST` | `localhost:8090` |
| reset on start | `INTERNAL_AI_COPILOT_STORE_RESET_ON_START` | `false` |
| AI profile | `INTERNAL_AI_COPILOT_AI_PROFILE` | 缺值時回退讀舊版 env |
| Gemini API key | `GEMINI_API_KEY` | 空 |
| OpenAI API key | `OPENAI_API_KEY` | 空 |

補充：
- `REWARDBRIDGE_*` legacy 前綴仍可 fallback。
- `AIPreviewMode` 舊 bool 仍存在，只在 `AI_PROFILE` 缺失且 `AIDefaultMode` 空時做 fallback。
- `AI_PROFILE` 現在是主 AI 與 promptguard 的主要共同開關：
  - `1 -> preview_full + promptguard cloud + main openai`
  - `2 -> preview_prompt_body_only + promptguard cloud + main openai`
  - `3 -> live + mock + promptguard cloud`
  - `4 -> live + openai + promptguard cloud`
  - `5 -> live + gemma + promptguard cloud`
  - `6 -> live + openai + promptguard local`
  - `7 -> live + gemma + promptguard local`
- `ResolvedAIModel()` 會依 provider 回 `OpenAIModel` 或 `GemmaModel`，而這兩個值現在可由 `AI_PROFILE` 直接灌入預設組合。
- 舊的 `AI_DEFAULT_MODE / AI_PROVIDER / PROMPTGUARD_*` 仍保留相容 fallback，但 README 與日常啟動方式已改為 `AI_PROFILE + API keys`。
- `GEMINI_API_KEY` 現在是主 Gemma 與 promptguard cloud 的共同主 key；若同時存在舊的 `INTERNAL_AI_COPILOT_GEMMA_API_KEY` / `INTERNAL_AI_COPILOT_PROMPTGUARD_API_KEY`，runtime 會優先採用 `GEMINI_API_KEY`。

## B. 查 Builder 列表

關鍵檔案：
- internal/gatekeeper/handler.go (line 33)
- internal/gatekeeper/usecase.go (line 31)
- internal/builder/query_service.go (line 16)
- internal/infra/store.go (line 206)
- internal/infra/store.go (line 224)

### HTTP / gRPC call chain

```text
HTTP GET /api/builders
  -> gatekeeper.Handler.listBuilders
  -> gatekeeper.UseCase.ListBuilders
  -> builder.QueryService.ListActiveBuilders
  -> store.ActiveBuildersContext

HTTP GET /api/external/builders
  -> gatekeeper.Handler.listExternalBuilders
  -> gatekeeper.UseCase.ListExternalBuilders
  -> guard.ValidateExternalApp
  -> builder.QueryService.ListActiveBuilders
  -> appAllowsBuilder filter

gRPC ListBuilders
  -> grpcapi.Service.ListBuilders
  -> appId empty ? public : external
```

### 實際資料讀法

```text
BuildersContext
  -> Firestore builders collection 全撈
  -> 依 builderId 排序

ActiveBuildersContext
  -> 在 Go 層過濾 builder.Active=true
```

## C. 一般 Consult

關鍵檔案：
- internal/gatekeeper/handler.go (line 42)
- internal/gatekeeper/usecase.go (line 57)
- internal/gatekeeper/service.go (line 72)
- internal/builder/consult_usecase.go (line 35)
- internal/builder/assemble_service.go (line 95)
- internal/output/service.go (line 21)

### 共享 call chain

```text
HTTP consult
  -> gatekeeper.Handler.consult / externalConsult
  -> parseConsultMultipart
  -> gatekeeper.UseCase.Consult / ExternalConsult
  -> guard.ValidateConsult / ValidateExternalConsult
  -> builder.ConsultUseCase.Consult

gRPC Consult
  -> grpcapi.Service.Consult
  -> gatekeeper.UseCase.Consult / ExternalConsult
  -> builder.ConsultUseCase.Consult
```

### gatekeeper 驗證規則

| 檢查 | 失敗碼 |
| --- | --- |
| client IP 空 | `CLIENT_IP_MISSING` |
| builderId 為 0 | `BUILDER_ID_MISSING` |
| builder 不存在 | `BUILDER_NOT_FOUND` |
| builder inactive | `BUILDER_INACTIVE` |
| outputFormat 非 markdown/xlsx | `UNSUPPORTED_OUTPUT_FORMAT` |
| 檔案數超過上限 | `FILE_COUNT_EXCEEDED` |
| 單檔超過上限 | `FILE_SIZE_EXCEEDED` |
| 總大小超過上限 | `FILE_TOTAL_SIZE_EXCEEDED` |
| 副檔名不支援 | `UNSUPPORTED_FILE_TYPE` |
| external app 缺值 / 不存在 / inactive | `APP_ID_MISSING` / `APP_NOT_FOUND` / `APP_INACTIVE` |
| builder 不在 app 白名單 | `APP_BUILDER_FORBIDDEN` |

### builder consult orchestration

```text
ConsultUseCase.Consult
   │
   ├─ goroutine A: 若沒有 PreloadedBuilder，讀 BuilderByIDContext
   ├─ goroutine B: 讀 SourcesByBuilderIDContext
   ├─ sources 空 -> SOURCE_ENTRIES_NOT_FOUND
   │
   ├─ foreach source where NeedsRagSupplement
   │   -> rag.ResolveUseCase.ResolveBySourceID
   │   -> 若需要 rag 但拿不到 -> RAG_SUPPLEMENTS_NOT_FOUND
   │
   ├─ assembleService.AssemblePrompt
   ├─ aiUseCase.Analyze
   └─ outputUseCase.Render
```

### prompt 組裝實際內容

```text
instructions
├─ framework header
│  └─ [EXECUTION_RULES] + JSON contract 說明
├─ [RAW_USER_TEXT]
├─ [SUBJECT_PROFILE]        // 只有 profile flow 才可能有
├─ [PROMPT_BLOCK-1..N]
│  └─ [SUPPLEMENT] title
└─ [USER_INPUT]             // 若 userText 沒被 override 吃掉

user message
└─ 固定字串：
   「請依 instructions 執行本次 consult，若有附件請一併納入分析。」
```

這代表：
- instructions 中仍保留 `[RAW_USER_TEXT]`，但 text 的 prompt injection / override guard 已改由上游 `gatekeeper -> promptguard -> Gemma/local` 先處理。
- live provider 看到的使用者 message 並不是前端原文。

## D. Profile Consult

關鍵檔案：
- internal/gatekeeper/handler.go (line 57)
- internal/gatekeeper/usecase.go (line 75)
- internal/gatekeeper/usecase.go (line 113)
- internal/gatekeeper/service.go (line 117)
- internal/builder/model.go (line 25)
- internal/builder/module_keys.go (line 18)
- internal/builder/weighted_entries.go (line 17)
- internal/builder/assemble_service.go (line 85)

### public HTTP 與 gRPC 差異

```text
HTTP /api/profile-consult
  -> 可帶 mode
  -> appId 只當 strategy hint
  -> 不做 external app auth

gRPC ProfileConsult
  -> appId 空     -> ValidateProfileConsult
  -> appId 非空   -> ValidateExternalProfileConsult
  -> response 只映射 status / statusAns / response
```

### subjectProfile 正規化規則

| 規則 | 失敗碼 |
| --- | --- |
| `subjectProfile=nil` | 合法 |
| `subjectId` 空且 `analysisPayloads` 空 | 當成沒傳 |
| `subjectId` 空但有 payload | `SUBJECT_ID_MISSING` |
| `analysisType` 空 / regex 不合 | `INVALID_MODULE_KEY` |
| `analysisType=common` | `RESERVED_MODULE_KEY` |
| `analysisType` 重複 | `DUPLICATE_ANALYSIS_PAYLOAD` |
| `theoryVersion` 有傳但 trim 後空白 | `THEORY_VERSION_MISSING` |
| weighted entry envelope 非法 | `INVALID_ANALYSIS_PAYLOAD` |
| text 與 normalized profile 都空 | `PROFILE_INPUT_EMPTY` |

### source filtering 實際分支

```text
ConsultModeProfile
   -> assembleService.FilterProfileSources
      -> resolveProfileContextStrategy(appId)
         -> no appId / no config / inactive config -> default
         -> strategyKey=linkchat                -> linkchat
         -> unknown strategyKey                 -> UNKNOWN_PROMPT_STRATEGY
```

default filtering:

```text
subjectProfile.analysisPayloads[].analysisType
   -> NormalizeStoredModuleKey
   -> tags set

foreach source
   -> NormalizeStoredModuleKey(source.ModuleKey)
   -> moduleKey 空 -> 保留
   -> moduleKey 在 tags -> 保留
```

linkchat filtering:

```text
foreach analysis payload
   -> renderer.SourceTags()
   -> astrology -> ["astrology"]
   -> mbti      -> ["mbti"]

foreach source
   -> moduleKey 空 -> 保留
   -> moduleKey 不在 tags -> 丟掉
   -> fragment source -> 丟掉
   -> primary source matchKey 若需要對應 slot key，未命中就丟掉
```

### LinkChat render 實際支援度

| analysisType | 目前 code 行為 |
| --- | --- |
| `astrology` | 走 composable source graph lookup |
| `mbti` | 只做 raw flatten render |
| 其他 | `UNSUPPORTED_ANALYSIS_TYPE` |

補充：
- `astrology` 會先把 payload flatten 成 weighted entries。
- `key` 是 canonical value。
- 多個 weighted entries 時，`weightPercent` 必填且總和必須為 100。
- `theoryVersion` 只會在 LinkChat render 的 `[SUBJECT_PROFILE]` 中保留，不進 lookup。

### `PromptBodyPreview` 的實際來源

`PromptBodyPreview` 不是整份 instructions 裁切後得到的。
它只來自 `profileBlock`，再把這些行去掉：
- `## [SUBJECT_PROFILE]`
- `### [analysis:*]`
- `theory_version:*`

因此 non-profile request 幾乎拿不到有內容的 prompt body preview。

## E. AI 執行層與 provider

關鍵檔案：
- internal/aiclient/service.go (line 35)
- internal/aiclient/provider_openai.go (line 28)
- internal/aiclient/provider_gemma.go (line 33)
- internal/infra/config.go (line 64)
- internal/app/app.go (line 52)

### mode 決策

```text
AnalyzeService.Analyze
  -> resolveAnalyzeMode(request.Mode)
     -> request mode valid ? 用 request
     -> else AIDefaultMode 非空 ? 用全域
     -> else AIPreviewMode=true ? preview_full
     -> else live

  -> preview_full               -> buildPreviewText
  -> preview_prompt_body_only   -> request.PromptBodyPreview
  -> live + AIMockMode=true     -> mockAnalyze
  -> live + AIMockMode=false    -> liveProvider().Analyze
```

### provider 細節

| provider | live API | 附件處理 | 結構化輸出 |
| --- | --- | --- | --- |
| OpenAI | `/responses` | 先傳 `/files`，image 用 `vision`，file 用 `user_data` | `text.format=json_schema` |
| Gemma | `/models/{model}:generateContent` | 先走 resumable upload，之後 `file_data.file_uri` | `generationConfig.responseJsonSchema` |

### business response contract

```json
{
  "status": true,
  "statusAns": "",
  "response": "給顧客看的結果",
  "responseDetail": "內部詳細分析"
}
```

preview / mock / live 都沿用這個 shape。

### mock path 目前的技術現況

`mockAnalyze` 目前只會：
1. 嘗試從 instructions 抓 `builderCode=...`
2. 若命中 `qa-smoke-doc` 才走專用表格模板
3. 否則走 general mock fallback

但目前正式 assemble path 不會把 `builderCode=` 放進 instructions。
所以 `qa-smoke-doc` 專用 mock 表格模板，現在通常不會靠正式 runtime 命中。

### provider-specific 錯誤碼

| provider | 失敗碼 |
| --- | --- |
| OpenAI key 缺值 | `OPENAI_API_KEY_MISSING` |
| OpenAI analyze 失敗 | `OPENAI_ANALYSIS_FAILED` |
| OpenAI 空 output | `OPENAI_EMPTY_OUTPUT` |
| OpenAI upload 失敗 / 被拒 | `ATTACHMENT_UPLOAD_FAILED` / `ATTACHMENT_UPLOAD_REJECTED` |
| Gemma key 缺值 | `GEMMA_API_KEY_MISSING` |
| Gemma analyze 失敗 | `GEMMA_ANALYSIS_FAILED` |
| Gemma 空 output | `GEMMA_EMPTY_OUTPUT` |
| Gemma upload 失敗 / 被拒 | `GEMMA_ATTACHMENT_UPLOAD_FAILED` / `GEMMA_ATTACHMENT_UPLOAD_REJECTED` |
| provider 未註冊 | `AI_PROVIDER_UNSUPPORTED` |

### PromptGuard module 現況

關鍵檔案：
- internal/promptguard/usecase.go (line 3)
- internal/promptguard/service.go (line 5)
- internal/promptguard/config.go (line 15)
- internal/app/app.go (line 27)
- internal/gatekeeper/usecase.go (line 79)
- internal/builder/assemble_service.go (line 149)
- internal/aiclient/guard_analyze.go (line 42)

目前 `promptguard` 的實際 runtime call chain 已經是：

```text
gatekeeper.UseCase.PublicProfileConsult / ProfileConsult
  -> evaluatePromptGuard
     -> shouldRunPromptGuard
        -> builderCode == linkchat-astrology
        -> text != ""
     -> promptguard.EvaluateUseCase.Evaluate
        -> promptguard.Service.Evaluate
           -> ScoreText
              ├─ allow -> 直接放行
              ├─ block -> 直接攔截
              └─ needs_llm
                 -> EvaluateWithLLM
                    -> builder.AssemblePromptGuard
                    -> aiUseCase.AnalyzeGuard
                       -> cloud or local Gemma
```

目前第一版 text classifier 真相：

| 步驟 | 目前行為 |
| --- | --- |
| `ScoreText` | 先做 normalize / match / score / route；normalize 目前包含去零寬字元、全半形收斂、空白收斂、lowercase；靜態 rule catalog 會先做 pattern/terms normalize 與 regex compile；`score=0` allow、`score>=8` block、其餘 `needs_llm` |
| `EvaluateWithLLM` | 若 builder assembler 或 llm route 沒 wiring，才回 placeholder `LLM_GUARD_CLOUD_PLACEHOLDER` / `LLM_GUARD_LOCAL_PLACEHOLDER` |
| `EvaluateWithLLM` 完整 wiring 後 | 會先組 dedicated guard prompt，再打 `aiclient.AnalyzeGuard` |
| `AnalyzeGuard status=true` | 映射成 `decision=allow` |
| `AnalyzeGuard status=false` | 映射成 `decision=block` |
| `mode` 缺失或非法 | fallback 到 `cloud` |

第一版 rule engine 目前固定用四類 category：
- `override_attempt`
- `prompt_exfiltration`
- `role_spoofing`
- `safety_bypass`

第一版 `Evaluation` 除了 `decision / score / reason / source`，也已經帶：
- `matchedRules`
- `matchedCategories`

目前主要啟動方式：

| 設定 | env |
| --- | --- |
| promptguard mode/profile | `INTERNAL_AI_COPILOT_AI_PROFILE` |
| promptguard cloud credential | `GEMINI_API_KEY` |
| promptguard local base URL | profile `6/7` 內建 `http://localhost:11434` |

目前最重要的 current truth：
- 它已經是獨立 module，不再只是規格文件。
- 它已經在 `app.New()` 被建起來，並注入 gatekeeper、builder、aiclient。
- 第一版只影響 `linkchat-astrology` 的 profile consult，generic consult 不受影響。
- 第一層 text scoring 已能直接攔截明顯 override / prompt leakage / role spoofing / safety bypass 類文字。
- 產品目前另外把 `提示詞`、`prompts`、`promots` 視為專案特化高風險詞；命中時第一層就直接 block，不再當成灰區 meta 詞送 Gemma。
- 若 catalog 內某條 regex rule 非法，現在會在初始化時直接停用該 rule，而不是在 request hot path 觸發 panic。
- `keyword` 與 `phrase` 在第一版 matcher 目前都走 substring matching；兩者差異暫時只在 rule weight 與 catalog 命名意圖。
- gatekeeper block 時不丟 validation 4xx，而是直接回正常 business response：`status=false`、`statusAns=prompts有違法注入內容`、`response=取消回應`。
- `AI_PROFILE` 合法時，promptguard 直接依 profile 決定 cloud/local、model 與 base URL；只有 profile 缺失或非法時才回退讀舊版 `PROMPTGUARD_*` env。
- 這條路現在真的可能因為外部 Gemma/local guard 失敗而把 request 當成 system error 擋下。
- `AnalyzeGuard` 現在對 promptguard JSON 做了最小容錯：若 Gemma 回 markdown code fence 或前後夾雜說明文字，會先嘗試清理並抽出第一段 JSON object；只有真的抽不出合法 JSON 時才回 502。

## F. Output 與 transport 回應

關鍵檔案：
- internal/output/service.go (line 21)
- internal/grpcapi/service.go (line 77)
- internal/grpcapi/service.go (line 123)
- internal/infra/types.go (line 75)

### output policy

```text
RenderService.Render
  -> response.File = nil
  -> Preview ? return
  -> !Status ? return
  -> !Builder.IncludeFile ? return
  -> resolve default output format
  -> request.OutputFormat 覆蓋 default
  -> markdown / xlsx renderer
  -> file bytes -> base64 -> response.File
```

### transport mapping

```text
HTTP
  -> infra.WriteJSON
  -> APIResponse{success,data,error}

gRPC Consult
  -> ConsultBusinessResponse.File.Base64
  -> decode -> pb.FilePayload.Data

gRPC ProfileConsult
  -> 只取 Status / StatusAns / Response
  -> File 與 ResponseDetail 都不映射
```

這代表 `responseDetail` 雖然是 business response contract 的一部分，但：
- HTTP 會照原樣回。
- gRPC generic consult 只回 status/statusAns/response/file。
- gRPC profile consult 更精簡，只回 status/statusAns/response。

## G. Admin Graph

關鍵檔案：
- internal/builder/admin_handler.go (line 35)
- internal/builder/graph_usecase.go (line 16)
- internal/builder/graph_service.go (line 33)
- internal/builder/query_service.go (line 38)
- internal/infra/store.go (line 361)

### LoadGraph call chain

```text
AdminHandler.loadGraph
  -> GraphUseCase.LoadGraph
  -> GraphService.LoadGraph
  -> QueryService.LoadGraph
     -> BuilderByIDContext
     -> SourcesByBuilderIDContext
     -> foreach source -> RagsBySourceIDContext
```

### SaveGraph 主要規則

| 規則 | 失敗碼 |
| --- | --- |
| builder 不存在 | `BUILDER_NOT_FOUND` |
| builderCode 空 | `BUILDER_FIELD_MISSING` |
| builderCode 撞到別人 | `BUILDER_CODE_DUPLICATE` |
| name 空 | `BUILDER_FIELD_MISSING` |
| defaultOutputFormat 非 markdown/xlsx | `UNSUPPORTED_OUTPUT_FORMAT` |
| sourceId 重複 | `SOURCE_ID_DUPLICATE` |
| orderNo <= 0 | `SOURCE_ORDER_INVALID` |
| sourceType 非 `primary/fragment` | `SOURCE_TYPE_INVALID` |
| moduleKey 非法 | `INVALID_MODULE_KEY` |
| sourceIds 指向不存在 request source | `SOURCE_REFERENCE_NOT_FOUND` |
| rag orderNo <= 0 | `RAG_ORDER_INVALID` |
| ragType 空 | `RAG_TYPE_MISSING` |
| rag retrievalMode 非 `full_context` | `RAG_RETRIEVAL_MODE_UNSUPPORTED` |

### `ReplaceBuilderGraph` 交易內實際步驟

```text
transaction
   │
   ├─ 讀 _meta/counters
   ├─ 讀 builder 下現有 sources
   ├─ 讀每個非 system source 的 sourceRags
   ├─ 刪掉所有非 system sources + rags + _sourceLookup
   ├─ 為新 sources 分配真實 sourceId
   ├─ 重寫 source.SourceIDs placeholder -> 真實 ID
   ├─ 寫入新 source
   ├─ 寫入 _sourceLookup
   ├─ 寫入對應 rag
   └─ 回寫 counters
```

補充：
- service 層先用負數 placeholder sourceID。
- transaction 內再把 placeholder 轉真實 ID。
- code 沒做 source graph 循環檢查。

## H. Admin Templates

關鍵檔案：
- internal/builder/admin_handler.go (line 68)
- internal/builder/template_usecase.go (line 16)
- internal/builder/template_service.go (line 36)
- internal/builder/query_service.go (line 98)
- internal/infra/store.go (line 555)
- internal/infra/store.go (line 635)

### builder-specific template list

```text
QueryService.ListTemplatesByBuilder
  -> BuilderByIDContext
  -> TemplatesContext 全撈
  -> 過濾 Active=true
  -> template.GroupKey != nil 時，必須和 builder.GroupKey 相等
  -> foreach template -> TemplateRagsByTemplateIDContext
```

### create / update / delete

```text
CreateTemplate
  -> normalizeAndPrepareTemplate(isCreate=true)
  -> store.SaveTemplate(transaction)
  -> store.ReorderTemplates
  -> query.ListAllTemplates 找回 response

UpdateTemplate
  -> 先確認 TemplateByIDContext 存在
  -> normalizeAndPrepareTemplate(isCreate=false)
  -> store.SaveTemplate(transaction)
  -> store.ReorderTemplates
  -> query.ListAllTemplates 找回 response

DeleteTemplate
  -> store.DeleteTemplate
```

### template 規則

| 規則 | 失敗碼 |
| --- | --- |
| templateKey 空 | `TEMPLATE_KEY_MISSING` |
| name 空 | `TEMPLATE_NAME_MISSING` |
| orderNo <= 0 | `TEMPLATE_ORDER_INVALID` |
| templateKey 重複 | `TEMPLATE_KEY_DUPLICATE` |
| template rag orderNo <= 0 | `TEMPLATE_RAG_ORDER_INVALID` |
| template rag type 空 | `TEMPLATE_RAG_TYPE_MISSING` |
| rag retrievalMode 非 `full_context` | `RAG_RETRIEVAL_MODE_UNSUPPORTED` |
| template 不存在 | `TEMPLATE_NOT_FOUND` |

### `DeleteTemplate` 的副作用

```text
DeleteTemplate
   ├─ 刪 templateRags
   ├─ 刪 template
   ├─ 走訪所有 builders
   │  └─ 清 copiedFromTemplateId / Key / Name / Description / GroupKey
   └─ 重新把剩餘 templates 的 orderNo 排成 1..N
```

這整段不是 transaction。
所以如果中間失敗，可能出現：
- template 已刪，但 source metadata 還沒清完
- 或 template 已刪，其他 templates 的 orderNo 尚未收斂

## I. 持久化模型、seed 基線、目前 gap

關鍵檔案：
- internal/infra/store.go (line 20)
- internal/infra/store.go (line 33)
- internal/infra/seed.go (line 4)
- internal/infra/http.go (line 16)

### Firestore collections

```text
apps/{appId}
appPromptConfigs/{appId}
builders/{builderId}
builders/{builderId}/sources/{sourceId}
builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}
templates/{templateId}
templates/{templateId}/templateRags/{templateRagId}
_meta/counters
_sourceLookup/{sourceId}
```

`_sourceLookup` 的用途是：
- 只知道 `sourceID` 時，先反查 `builderID`
- 再定位到正確的 builder subcollection source doc

### startup seed 真相

目前 `DefaultSeedData()` 會種下：
- 1 個 app：`linkchat`
- 1 個 appPromptConfig：`linkchat -> linkchat`
- 3 個 builders：`pm-estimate`、`qa-smoke-doc`、`linkchat-astrology`
- 4 個 templates
- 32 個 sources
- 4 個 source rags

### reset / seed 規則

```text
ResetOnStart=true
   -> 清 builders/apps/appPromptConfigs/templates/_sourceLookup/_meta
   -> 全量重種 seed

SeedWhenEmpty=true
   -> 只有 builders 與 templates 都空時才全量 seed
   -> 之後仍會 ensureApps
```

這表示：
- local/dev 可以靠 emulator reset 回到固定狀態。
- 非空 store 啟動時，不會重建 builders/templates，但會補 app 資料。

### 目前最明確的 gap / 風險

| 項目 | 目前狀態 |
| --- | --- |
| admin auth | 未實作 |
| public dev routes 防護 | 未實作 |
| app prompt config cache invalidation | 未實作 |
| source graph cycle detection | 未實作 |
| template delete transaction | 未實作 |
| mbti seed graph | 未落地 |
| builder-specific mock 分支 | 程式有寫，但正式 prompt 沒餵 `builderCode=` marker |

### HTTP / gRPC 回應基線

HTTP：

```json
{
  "success": true,
  "data": {},
  "error": null
}
```

gRPC：
- 會把 business error 的 HTTP status 映射成對應 gRPC code。
- `ErrorInfo.Reason` 會帶 business error code。

對應檔案：
- internal/grpcapi/service.go (line 162)
- internal/infra/http.go (line 23)
