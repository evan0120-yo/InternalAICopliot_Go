# Builder BDD Spec

## Purpose
這份文件定義 builder module 目前應滿足的行為規格。內容以現有 code、測試與已存在的 module spec 為基準；未被 code 穩定證實的部分必須列為 open question。

## Actors
- gatekeeper usecase：將通過驗證的 consult 請求交給 builder
- admin user：透過 admin API 讀取或修改 graph / template
- builder module：負責 consult orchestration、graph save/load、template CRUD 與 prompt assembly

## Scenario Group: Consult Orchestration
```text
Gatekeeper -> ConsultUseCase
        │
        ├─ load builder + sources
        │
        ├─ mode = generic
        │   └─ 載入全量 eligible sources
        │
        └─ mode = profile
            ├─ 保留 builder source/rag 骨架
            ├─ appId=""        -> default strategy
            └─ appId=linkchat  -> LinkChat strategy
                    └─ 第二層 analysis factory 決定
                       哪些 internal keys / source tags 參與
```

- Given builder 存在且 source 可載入
  When `ConsultUseCase.Consult` 被呼叫
  Then 系統應載入 builder 與 sources，必要時為各 source resolve RAG，組裝 prompt，呼叫 AI，再交給 output module 決定是否產檔

- Given request mode 為 `ConsultModeGeneric`
  When consult 開始執行
  Then 系統應依 generic consult 規則載入全量 eligible sources

- Given request mode 為 `ConsultModeProfile` 且 `appId=linkchat`
  When consult 開始執行
  Then 系統應保留同一個 builder 的整體 source/rag 骨架，並將 analysis-specific 解析責任交給 LinkChat strategy

- Given LinkChat request 只送出 `analysisType=astrology`
  When consult 開始執行
  Then builder 不應自行補回 `mbti` 或其他未送入的 analysis type

- Given request mode 為 `ConsultModeProfile` 且 `subjectProfile` 缺值但 `text` 有值
  When consult 開始執行
  Then 系統不應回退成 generic consult

- Given builder 不存在
  When consult 開始執行
  Then 應回傳 `BUILDER_NOT_FOUND`

- Given source 查詢為空
  When consult 開始執行
  Then 應回傳 `SOURCE_ENTRIES_NOT_FOUND`

- Given request context 已取消或逾時
  When consult 任一階段檢查到 cancellation
  Then 應回傳 `REQUEST_CANCELLED`

- Given 某個 source 標記 `NeedsRagSupplement=true`
  When rag module 沒有回傳任何 supplements
  Then 應回傳 `RAG_SUPPLEMENTS_NOT_FOUND`

## Scenario Group: Prompt Assembly
```text
AssemblePrompt
      │
      ├─ framework header
      ├─ [RAW_USER_TEXT]
      ├─ app-aware profile/context block
      │   ├─ default
      │   └─ linkchat
      │       ├─ astrology factory
      │       └─ mbti factory
      ├─ selected sources
      │   └─ resolved rags
      ├─ [USER_INPUT]（若未被 override 消化）
      └─ framework tail
```

- Given sources 與 rags 已就緒
  When `AssembleService.AssemblePrompt` 執行
  Then prompt 必須先按 source order 排序，再輸出每個 source 區塊與其 RAG 區塊，最後補上 framework tail

- Given request 的 `appId` 為空
  When `AssembleService.AssemblePrompt` 執行
  Then 系統應使用 default prompt strategy

- Given request 的 `appId` 為 `linkchat`
  When `AssembleService.AssemblePrompt` 執行
  Then 系統應使用 LinkChat prompt strategy，但仍共用相同的 framework header、source/rag 排序與 framework tail

- Given `appId=linkchat` 且 payload 內帶 `analysisType=astrology`
  When `AssembleService.AssemblePrompt` 執行
  Then LinkChat strategy 應選出對應的 astrology analysis factory 來解讀 payload 與組裝 prompt fragment

- Given `appId=linkchat` 且 payload 內帶 `analysisType=mbti`
  When `AssembleService.AssemblePrompt` 執行
  Then LinkChat strategy 應選出對應的 mbti analysis factory，而不要求與 astrology 共用同一套 payload shape

- Given structured `subjectProfile` 有值
  When `AssembleService.AssemblePrompt` 執行
  Then prompt 應包含 deterministic 的 `[SUBJECT_PROFILE]` 區塊，用來承接 external app 傳來的 analysis payloads

- Given structured `subjectProfile` 有值
  When `AssembleService.AssemblePrompt` 執行
  Then `[SUBJECT_PROFILE]` 區塊必須出現在 `[RAW_USER_TEXT]` 之後、第一個 source block 之前

- Given `subjectProfile` 內多個 analysis payload 的輸入順序不同
  When `[SUBJECT_PROFILE]` 被 render
  Then 各 analysis payload 的輸出必須 deterministic，且由對應 analysis factory 決定內部排序規則

- Given LinkChat strategy 遇到走 canonical key composable path 的 analysis type
  When app-aware profile/context block 被 render
  Then 應先依 slot key 選 primary source
  And 應再依 canonical value 找到對應 fragment source
  And 應由 Internal 直接展開 source graph 後輸出最終 prompt

- Given 某個 analysis type 沒有走 canonical key composable path
  When app-aware profile/context block 被 render
  Then 應保留原始值，不強制轉成其他 lookup key

- Given `appId=linkchat` 且 `analysisType=astrology`
  When app-aware profile/context block 被 render
  Then 該 analysis type 應視為 canonical-key composable analysis
  And Internal 應直接以 canonical value 對 `source.matchKey` 做 lookup

- Given `appId=linkchat` 且 `analysisType=mbti`
  When app-aware profile/context block 被 render
  Then 該 analysis type 目前可保留 raw value render
  And 不要求使用 canonical-key composable graph

- Given LinkChat strategy 已完成 canonical key source resolution
  When profile/context block 被插入 shared prompt skeleton
  Then AI 應只看到翻譯後的語意結果
  And 不應直接看到 raw theory 詞、canonical key 或 Internal private code

- Given LinkChat 之後需要以 `analysisType` / slot key / value key 做更細的語意片段組裝
  When builder 執行 LinkChat prompt strategy
  Then 這些 key 的解析與拼接責任應落在 strategy 內，而不是改變既有 source / RAG 的主結構

- Given source 與 rag collections 已存在固定結構
  When LinkChat strategy 使用自己的 key system 做 prompt 組裝
  Then source 仍應作為主 prompt 骨架，rag 仍應作為 source 底下的補充內容

- Given某個 value 內含 `|`
  When `[SUBJECT_PROFILE]` 被 render
  Then 該 value 內的 `|` 應 escape 成 `\|`

- Given user text 為空
  When prompt 組裝完成
  Then `[RAW_USER_TEXT]` 區塊要以預設文字描述「用戶沒有額外需求」

- Given某個 overridable rag 含有 `{{userText}}`
  When user text 有值
  Then 應先以 user text 替換 placeholder，且不再追加 `[USER_INPUT]` 區塊

- Given某個 overridable rag 不含 placeholder
  When user text 有值
  Then 該 rag 內容應直接被 user text 覆寫，且不再追加 `[USER_INPUT]` 區塊

- Given沒有任何 override 消化 user text
  When user text 有值
  Then prompt 應追加 `[USER_INPUT]` 區塊

## Scenario Group: Graph
```text
SaveGraph
   │
   ├─ legacy aiagent[] ? -> 先轉 sources[]
   ├─ normalize builder config
   ├─ normalize source.moduleKey
   ├─ validate rag / order / output format
   ├─ merge builder config
   └─ replace non-system sources + persist
```

- Given admin 載入既有 builder graph
  When `LoadGraph` 執行
  Then 應回傳 builder 設定、sources 與每個 source 底下的 rags

- Given admin 儲存 graph
  When `SaveGraph` 執行
  Then builder 設定應先 merge 後保存，非 system source 應重新 canonical reorder，system block 必須保留

- Given graph request 使用 legacy `aiagent[]` source 形狀
  When `SaveGraph` 執行
  Then 系統應將其轉成 sources 後續處理

- Given graph rag request 使用舊欄位 `prompts`
  When rag content 為空
  Then 應使用 `prompts` 作為 rag content alias

- Given graph source request 帶 `moduleKey`
  When graph 儲存與後續讀取
  Then source 的 `moduleKey` 應被正規化為 canonical lowercase key

- Given graph source request 的 `moduleKey` 為 `common`、空字串或缺值
  When graph 儲存
  Then source 應被視為 common source，並以缺值 / 空值形式保存

- Given graph source request 的 `moduleKey` 不符合 `^[a-z0-9][a-z0-9_-]*$`
  When graph 儲存
  Then 應拒絕儲存並回傳 invalid module key error

- Given builder 的 `groupKey` 為空但 `groupLabel` 有值
  When graph 儲存 builder 設定
  Then 系統應從 `groupLabel` 衍生非空的 slug 形式 group key

- Given builderCode 被指定為空字串或與其他 builder 重複
  When graph merge builder 設定
  Then 應拒絕儲存並回傳 builder validation error

- Given builder default output format 不是 `markdown` 或 `xlsx`
  When graph merge builder 設定
  Then 應拒絕儲存並回傳 `UNSUPPORTED_OUTPUT_FORMAT`

- Given source `orderNo` 或 rag `orderNo` 小於等於 0
  When graph 正規化執行
  Then 應拒絕儲存並回傳對應 invalid order error

- Given rag type 缺失或 retrieval mode 不是 `full_context`
  When graph 正規化執行
  Then 應拒絕儲存並回傳 validation error

## Scenario Group: Templates
```text
Template command
   │
   ├─ validate templateKey / name / order
   ├─ normalize rag config
   ├─ create / update / reorder
   └─ delete 時清 template references
```

- Given admin 建立 template
  When request 合法
  Then `templateKey` 必須唯一、template name 必填、order 應 canonicalize，並回傳建立後的 template 與 rags

- Given admin 建立或更新 template 時 `templateKey` 缺失、重複，或 `name` 缺失
  When template normalization 執行
  Then 應拒絕儲存並回傳對應 validation error

- Given template rag type 缺失、rag `orderNo` 非正整數，或 `retrievalMode` 不是 `full_context`
  When template normalization 執行
  Then 應拒絕儲存並回傳對應 validation error

- Given admin 更新 template
  When request 指定新的 `orderNo`
  Then 既有 template 應移動到指定位置，其他 template order 順延

- Given admin 刪除 template
  When template 存在
  Then template 與其 template rags 應被刪除，且 source 上所有 copied-from-template references 都要清空

- Given builder 查詢可用 templates
  When `ListTemplatesByBuilder` 執行
  Then 只回傳 active templates，且若 template 有 `groupKey`，必須與 builder `groupKey` 相符

## Acceptance Notes
- builder consult 最終必須回傳 `infra.ConsultBusinessResponse`
- builder graph 與 template 的 admin HTTP route 由 `admin_handler.go` 暴露
- builder 對 rag 的既定讀取模式目前只接受 `full_context`
- LinkChat profile-analysis 第一版採單一 builder + 兩層工廠；不為每種理論各自建立一個 builder
- `ConsultModeProfile` 與 `ConsultModeGeneric` 必須由 transport / gatekeeper 明確決定，不可由 builder 自行猜測
- prompt assembly 的 app-aware 差異應落在 shared assembly skeleton 內部的 strategy，而不是複製整條 consult flow
- strategy registry 目前沒有 TTL / invalidation；更新 Firestore 後需重啟服務才保證吃到最新值

## Code-Backed Tests
- `consult_usecase_test.go`
- `assemble_service_test.go`
- `graph_service_test.go`
- `template_service_test.go`

## Open Questions
- consult orchestration 目前使用 `sync.WaitGroup` 與 goroutine；未來是否改為 `errgroup` 尚未定案
- template 與 graph 的 handler 層尚未有完整 HTTP 測試，部分驗收目前仍由 service / usecase 測試間接保護
- `ProfileConsult` 與 app-aware strategy 已落成 production code；canonical-key composable source graph 與移除 theoryMappings 是下一階段 implementation target

## Scenario Group: Composable Source Graph For Astrology

```text
LinkChat payload
  ├─ sun_sign=...
  ├─ moon_sign=...
  └─ rising_sign=...
        │
        ▼
LinkChat analysis parser
  ├─ 先決定主順序
  │  例如 sun -> moon -> rising
  │
  ├─ 每個 slot 選一個 primary source
  │
└─ 每個 canonical value 直接對應
   fragment source.matchKey
        │
        ▼
primary source
  ├─ sourceIds[] 依填入順序展開 child sources
  └─ 每個 source 再帶自己的 rag children
```

設計約束：
- source 應新增：
  - `sourceType` (`primary` / `fragment`)
  - `matchKey`
  - `sourceIds[]`
- primary source 順序由 analysis parser / request JSON 語意決定，不由 flat source order 自動推導
- `sourceIds[]` 陣列順序即 child expansion 順序，不做額外排序
- rag ownership 維持不變：
  - rag 仍屬於自己的 source
  - 不改成 `source -> rag pool`
- LinkChat 應先把 raw value / alias 正規化成 canonical value
- Internal 應直接以 canonical value lookup fragment source.matchKey
  - prompt 片段本身放在 source / rag graph
- 第一版先不要求：
  - source graph 防循環
  - 跨鏈去重
