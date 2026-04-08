# Builder Persona Notes And Tags BDD Spec

## Purpose
這份文件定義 builder module 為支撐 LinkChat persona_notes 與 persona_tags 兩種新 analysisType 而需要滿足的行為規格。

這份 BDD 是 Internal AI Copilot Go Backend 對應 LinkChat PERSONA_NOTES_TAGS_BDD / SDD 的接收端行為規格。

## Scope
- In Scope：
  - `assemble_service.go` 內 LinkChat strategy 新增 `persona_notes` 與 `persona_tags` renderer
  - prompt render 格式與錯誤處理
  - Internal 端對 payload 的基本驗證
- Out Of Scope：
  - gatekeeper 驗證規則（現有已可接受）
  - builder model 結構變更（現有已可承接）
  - LinkChat 端資料模型、UI、tag catalog 管理
  - 收費、解鎖、訂閱 gating
  - Firestore source data 新增（兩者都不走 source graph）

## Actors
- LinkChat backend：透過 gRPC ProfileConsult 傳入包含 `persona_notes` 或 `persona_tags` 的 `subjectProfile`
- builder module：負責將新 analysisType 正確 render 成 prompt block
- 使用者：在 LinkChat 端編輯人物補充三句與 tag（含自定義的 tag 意義說明）

## Core Design Decision

```text
persona_notes 與 persona_tags 都是使用者自己寫的人物背景資料，
本質上和 mbti 類似：payload 進來就是最終要 render 的內容。

  persona_notes
  ├─ payload["lines"] = 使用者寫的原文短句
  └─ 直接 render，不走 source graph

  persona_tags
  ├─ payload["selected"] = 使用者選的 tag + 使用者定義的意義說明
  └─ 直接 render，不走 source graph

兩者都不需要 Internal 做 canonical key → fragment 映射。
prompts 從 LinkChat 傳過來，Internal 直接組裝。
```

## Prerequisite Validation
以下文件已確認 Internal 現有架構可直接接受 persona_notes / persona_tags：

```text
gatekeeper 驗證鏈
    │
    ├─ NormalizeAnalysisTypeKey("persona_notes") → ✅ 通過 ^[a-z0-9][a-z0-9_-]*$
    ├─ NormalizeAnalysisTypeKey("persona_tags")  → ✅ 通過 ^[a-z0-9][a-z0-9_-]*$
    ├─ ValidateWeightedPayloadEnvelope           → ✅ 可接受 lines: []string 與 selected: [{...}]
    └─ SubjectAnalysisPayload.Payload map        → ✅ 已支援 map[string]any
```

## Scenario Group: LinkChat Analysis Renderer Factory

```text
linkChatAnalysisRenderer(analysisType)
    │
    ├─ "astrology"      → ✅ 既有
    ├─ "mbti"           → ✅ 既有
    ├─ "persona_notes"  → ❌ 需新增
    ├─ "persona_tags"   → ❌ 需新增
    └─ 其他             → UNSUPPORTED_ANALYSIS_TYPE
```

- Given `appId=linkchat` 且 payload 內帶 `analysisType=persona_notes`
  When `linkChatAnalysisRenderer` 被呼叫
  Then 應回傳 persona_notes renderer 而不是回傳 `UNSUPPORTED_ANALYSIS_TYPE`

- Given `appId=linkchat` 且 payload 內帶 `analysisType=persona_tags`
  When `linkChatAnalysisRenderer` 被呼叫
  Then 應回傳 persona_tags renderer 而不是回傳 `UNSUPPORTED_ANALYSIS_TYPE`

## Scenario Group: Persona Notes Renderer

```text
persona_notes renderer
    │
    ├─ AnalysisType() → "persona_notes"
    ├─ SourceTags()   → ["persona_notes"]
    └─ Build()
        ├─ 讀 payload["lines"] → []string
        ├─ trim 每條、移除空字串
        ├─ 驗證 1~3 條（超過回錯）
        ├─ 保留原始順序
        └─ render:
           ### [analysis:persona_notes]
           note_1: 慢熟，剛開始不太主動聊天
           note_2: 壓力大時會先自己消化
           note_3: 如果先給步驟，會比較願意配合
```

- Given `analysisType=persona_notes` 且 payload 包含合法 lines 陣列
  When persona_notes renderer `Build()` 執行
  Then 應讀取 `payload["lines"]` 並 render 成 deterministic profile block

- Given persona_notes payload 包含 3 條合法短句
  When renderer render 時
  Then 應以 `note_1`、`note_2`、`note_3` 作為 key
  And 順序必須與 LinkChat 傳入順序一致

- Given persona_notes payload 包含 1 條合法短句
  When renderer render 時
  Then 應只產生 `note_1` 一行

- Given persona_notes payload 的 `lines` 為空陣列
  When renderer `Build()` 執行
  Then 應回傳空 block（不產生 prompt 內容）

- Given persona_notes payload 的 `lines` 超過 3 條（扣除空字串後）
  When renderer `Build()` 執行
  Then 應回傳驗證錯誤 `PERSONA_NOTES_LIMIT_EXCEEDED`

- Given persona_notes payload 缺少 `lines` key
  When renderer `Build()` 執行
  Then 應回傳空 block（不產生 prompt 內容）

- Given persona_notes payload 的 `lines` 中有空字串
  When renderer `Build()` 執行
  Then 應 trim 每條後移除空字串，只 render 非空的短句

- Given persona_notes renderer 被呼叫 `SourceTags()`
  When source filtering 執行
  Then 應回傳 `["persona_notes"]`

- Given persona_notes 已 render 完成
  When prompt 最終組裝
  Then notes 內容應進入 `[SUBJECT_PROFILE]` block
  And 不得被併入 `[RAW_USER_TEXT]`

- Given persona_notes 內的某條短句帶有命令語氣
  When prompt 組裝完成
  Then 該內容仍應被 render 在 `[SUBJECT_PROFILE]` 內
  And 不應改變 `text` 的原始使用者輸入語意

## Scenario Group: Persona Tags Renderer

```text
persona_tags renderer
    │
    ├─ AnalysisType() → "persona_tags"
    ├─ SourceTags()   → ["persona_tags"]
    └─ Build()
        ├─ 讀 payload["selected"] → []map{groupKey, tagKey, prompt}
        ├─ 驗證每個 item 具備 groupKey、tagKey、prompt
        ├─ 跳過缺值項
        ├─ 同一個 groupKey 多個 tagKey → 分行列出
        └─ render:
           ### [analysis:persona_tags]
           role: 這個對象更適合用教學式、步驟式、降低壓力的方式互動
           communication_style: 先暖身、先降低防備，再進主題
```

- Given `analysisType=persona_tags` 且 payload 包含合法 selected 陣列
  When persona_tags renderer `Build()` 執行
  Then 應讀取 `payload["selected"]` 並直接以 LinkChat 傳來的 prompt 文字 render

- Given persona_tags payload 包含 `{groupKey: "role", tagKey: "student", prompt: "教學式互動..."}`
  When renderer render 時
  Then 應以 `groupKey` 作為 render 的 key
  And 應以 `prompt` 作為 render 的 value
  And 不需要去 source graph 或 Firestore 查詢 fragment

- Given persona_tags payload 中同一個 groupKey 有多個 tagKey（multi-select）
  When renderer render 時
  Then 每個 tag 應分行列出
  And 可以同一個 groupKey 出現多次

- Given persona_tags payload 的 `selected` 為空陣列
  When renderer `Build()` 執行
  Then 應回傳空 block（不產生 prompt 內容）

- Given persona_tags payload 缺少 `selected` key
  When renderer `Build()` 執行
  Then 應回傳空 block（不產生 prompt 內容）

- Given persona_tags payload 的某個 item 缺少 `groupKey` 或 `tagKey` 或 `prompt`
  When renderer `Build()` 執行
  Then 應跳過該 item

- Given persona_tags renderer 被呼叫 `SourceTags()`
  When source filtering 執行
  Then 應回傳 `["persona_tags"]`

- Given persona_tags 的 prompt 文字由 LinkChat 傳來
  When renderer render 時
  Then Internal 應原文使用，不做摘要、重寫或翻譯

## Scenario Group: Source Filtering

- Given LinkChat request 同時帶有 `astrology`、`persona_notes` 與 `persona_tags`
  When `FilterProfileSources` 執行
  Then 三種 analysisType 對應的 source 都應被保留

- Given LinkChat request 只帶 `persona_notes`
  When `FilterProfileSources` 執行
  Then 只有 `moduleKey=persona_notes` 的 source 與 common source 被保留
  And `moduleKey=astrology` 的 source 應被過濾掉

- Given LinkChat request 只帶 `persona_tags`
  When `FilterProfileSources` 執行
  Then 只有 `moduleKey=persona_tags` 的 source 與 common source 被保留

## Scenario Group: Prompt Render Format

```text
完整 prompt 範例（三種 analysisType 同時存在時）：

  ## [SUBJECT_PROFILE]

  ### [analysis:astrology]
  theory_version: astro_v1
  主執行緒...: <展開後語意片段>

  ### [analysis:persona_notes]
  note_1: 慢熟，剛開始不太主動聊天
  note_2: 壓力大時會先自己消化
  note_3: 如果先給步驟，會比較願意配合

  ### [analysis:persona_tags]
  role: 這個對象更適合用教學式、步驟式、降低壓力的方式互動
  communication_style: 先暖身、先降低防備，再進主題
```

- Given 三種 analysisType 同時存在
  When `[SUBJECT_PROFILE]` block 被 render
  Then 三個 analysis block 都應出現
  And 各 block 應依 analysisType 字母排序（deterministic）

- Given persona_notes render 結果
  When `[SUBJECT_PROFILE]` block 被 render
  Then `theory_version` 行不應出現（persona_notes 不使用 theoryVersion）

- Given persona_tags render 結果
  When `[SUBJECT_PROFILE]` block 被 render
  Then `theory_version` 行不應出現（persona_tags 不使用 theoryVersion）

- Given persona_tags 中同一 groupKey 有兩個 tag
  When `[SUBJECT_PROFILE]` block 被 render
  Then 應 render 成兩行，例如：
  ```
  role: 教學式互動...
  role: 同伴式互動...
  ```

## Scenario Group: Error Cases

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|----------|
| UNSUPPORTED_ANALYSIS_TYPE | 400 | linkChatAnalysisRenderer 收到未知 analysisType |
| PERSONA_NOTES_LIMIT_EXCEEDED | 400 | persona_notes 的 lines 超過 3 條 |
| INVALID_SOURCE_MODULE_KEY | 500 | 沿用既有錯誤碼，source 的 moduleKey 格式非法 |

- Given persona_notes 的 lines 超過 3 條（扣除空字串後）
  When renderer `Build()` 執行
  Then 應回傳 `PERSONA_NOTES_LIMIT_EXCEEDED`

## Acceptance Notes
- 兩個新 renderer 都應實作 `linkChatAnalysisRenderer` interface
- persona_notes 類似 mbti 的 flatten 模式，直接 render lines
- persona_tags 也是 flatten 模式，直接 render LinkChat 傳來的 prompt 文字
- **兩者都不走 source graph lookup**
- **兩者都不需要新增 Firestore source data**
- 不需要修改 gatekeeper、builder/model.go、builder/module_keys.go
- Internal 端對 persona_notes 做 max 3 條驗證（不只信任 LinkChat 端）
- persona_tags 的 prompt 文字來自 LinkChat（使用者在 LinkChat 端為 tag 定義的意義說明）

## Code-Backed Tests
預期新增測試：
- `assemble_service_test.go` — persona_notes renderer 單元測試
- `assemble_service_test.go` — persona_tags renderer 單元測試
- `assemble_service_test.go` — source filtering 包含新 analysisType

## Open Questions
- persona_tags 的 payload 中 prompt 文字的欄位名稱是否確定叫 `prompt`？還是 `description` 或其他命名？
- persona_tags 的同一 groupKey 多個 tag 分行時，是否需要對 tag 做排序？還是保留 LinkChat 傳入順序？
