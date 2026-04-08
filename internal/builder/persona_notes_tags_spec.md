# Builder Persona Notes And Tags Spec

## Purpose
這份文件定義 builder module 為支撐 LinkChat `persona_notes` 與 `persona_tags` 兩種新 analysisType 的設計規格。

這是 Internal AI Copilot Go Backend 的接收端規格，對應 LinkChat 端的 PERSONA_NOTES_TAGS_SDD。

此文件用於模組層級的設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
LinkChat 新增了兩種人物背景資料類型：

- **persona_notes**：使用者為對象寫的最多三條補充短句
- **persona_tags**：使用者為對象選擇的標籤，每個 tag 帶有使用者自定義的意義說明

這兩種資料都是使用者自己產生的內容，透過既有的 `subjectProfile.analysisPayloads` 傳入 Internal。Internal 只需直接組裝成 prompt，不需要做任何 source graph lookup 或 fragment 映射。

## Core Design Decision

```text
為什麼 persona_notes / persona_tags 不走 source graph：

  ├─ 兩者的內容都是使用者自己寫的
  │  ├─ notes = 使用者寫的原文短句
  │  └─ tags = 使用者選的 tag + 使用者定義的意義說明（prompt 文字）
  │
  ├─ LinkChat 保存這些內容
  │  └─ 發分析請求時把 prompt 文字一起傳給 Internal
  │
  ├─ Internal 直接 render
  │  ├─ 不需要 canonical key → fragment 映射
  │  ├─ 不需要 Firestore source data
  │  └─ 不需要 buildSourceGraphIndex / expandComposableSource
  │
  └─ 與 astrology 的本質差異：
     ├─ astrology 的語意解釋由 Internal 的 source graph 定義
     └─ notes / tags 的語意由使用者直接提供
```

## Architecture Fit

```text
現有架構擴展點：

  HTTP / gRPC Request (profile-consult)
    ↓
  gatekeeper.ValidateProfileConsult          ← ✅ 不需改
    ↓
  builder.ConsultUseCase.Consult
    ├─ FilterProfileSources                  ← ✅ 不需改，renderer.SourceTags() 驅動
    │  └─ resolveProfileContextStrategy
    │     └─ appId="linkchat" → linkChatProfileContextStrategy
    ↓
  assemble_service.AssemblePrompt
    ├─ strategy.Build()
    │  └─ linkChatAnalysisRenderer(analysisType)
    │     ├─ "astrology"      → ✅ 既有（走 source graph）
    │     ├─ "mbti"           → ✅ 既有（直接 flatten）
    │     ├─ "persona_notes"  → ❌ 需新增（直接 flatten，類似 mbti）
    │     └─ "persona_tags"   → ❌ 需新增（直接 flatten，類似 mbti）
    ↓
  prompt 組裝 → AI 呼叫 → 回傳
```

核心結論：只需在 `assemble_service.go` 新增兩個 renderer，兩個都用直接 flatten render 模式。不需要新增 Firestore data、不需要改架構。

## Change Summary

### 必改項目

| 改動項目 | 檔案 | 改動量 | 說明 |
|---------|------|--------|------|
| 新增 2 個 renderer struct | assemble_service.go | 中 | personaNotesLinkChatAnalysisRenderer + personaTagsLinkChatAnalysisRenderer |
| 新增 2 個 switch case | assemble_service.go | 小 | linkChatAnalysisRenderer 的 switch 新增 persona_notes / persona_tags |
| 新增錯誤碼 | assemble_service.go | 小 | PERSONA_NOTES_LIMIT_EXCEEDED |

### 不需改動的項目

| 模組 | 原因 |
|------|------|
| gatekeeper/handler.go | request 結構不變 |
| gatekeeper/service.go | normalizeSubjectProfile 已能處理 |
| builder/model.go | SubjectAnalysisPayload 結構不變 |
| builder/module_keys.go | pattern 驗證已支援 |
| builder/weighted_entries.go | 不使用 weighted entry |
| builder/consult_usecase.go | orchestration 流程不變 |
| aiclient/ | 不需改 |
| output/ | 不需改 |
| rag/ | 不需改 |
| grpcapi/ | 不需改 |
| infra/types.go | 不需改 |
| Firestore source data | 不需新增（不走 source graph） |

## Persona Notes Renderer Spec

### Renderer Interface Implementation

```text
personaNotesLinkChatAnalysisRenderer struct{}

  ├─ AnalysisType() → "persona_notes"
  │
  ├─ SourceTags(payload) → []string{"persona_notes"}
  │
  └─ Build(ctx, service, builderConfig, appID, payload)
       ├─ 讀取 payload.Payload["lines"]
       │  └─ 型別：[]any → 逐項轉 string
       ├─ 前處理：
       │  ├─ trim 每條
       │  └─ 移除 trim 後的空字串
       ├─ 驗證：
       │  ├─ 0 條 → 回傳空 block
       │  └─ 超過 3 條 → 回傳 PERSONA_NOTES_LIMIT_EXCEEDED
       ├─ 保留原始順序
       └─ 組裝 renderedAnalysisBlock
```

### Render 行為

persona_notes renderer 的設計與 mbti renderer 相同：讀 payload、flatten、render。

```text
不走 source graph 的原因：
├─ notes 是使用者自由文字
├─ 內容即 prompt，不需要 Internal 做映射
├─ notes 的價值在於原文呈現
└─ 直接 render lines，不做語意轉換
```

### Render 格式

```text
### [analysis:persona_notes]
note_1: 慢熟，剛開始不太主動聊天
note_2: 壓力大時會先自己消化
note_3: 如果先給步驟，會比較願意配合
```

key 命名規則：
- 使用 `note_{index}` 作為 key，index 從 1 開始
- 順序必須與 LinkChat 傳入順序一致
- 不對內容做摘要、重寫、翻譯或合併

### Internal 端驗證

Internal 端必須自己驗證 lines 不超過 3 條（不只信任 LinkChat 端已做驗證）。

```text
驗證流程：
  lines = payload["lines"]
    ├─ 逐項 trim
    ├─ 移除空字串
    ├─ 剩餘 0 條 → 空 block
    ├─ 剩餘 1~3 條 → 正常 render
    └─ 剩餘 > 3 條 → PERSONA_NOTES_LIMIT_EXCEEDED (400)
```

### Payload Shape

```json
{
  "analysisType": "persona_notes",
  "payload": {
    "lines": [
      "慢熟，剛開始不太主動聊天",
      "壓力大時會先自己消化",
      "如果先給步驟，會比較願意配合"
    ]
  }
}
```

### persona_notes 不使用 theoryVersion

persona_notes payload 不帶 `theoryVersion`。renderer 應接受 `theoryVersion=nil`，render 結果不應包含 `theory_version` 行。

## Persona Tags Renderer Spec

### Renderer Interface Implementation

```text
personaTagsLinkChatAnalysisRenderer struct{}

  ├─ AnalysisType() → "persona_tags"
  │
  ├─ SourceTags(payload) → []string{"persona_tags"}
  │
  └─ Build(ctx, service, builderConfig, appID, payload)
       ├─ 讀取 payload.Payload["selected"]
       │  └─ 型別：[]any → 逐項轉 map[string]any
       ├─ 對每個 item：
       │  ├─ 讀 groupKey (string) — 必填
       │  ├─ 讀 tagKey (string) — 必填
       │  ├─ 讀 prompt (string) — 必填
       │  └─ 缺任一必填欄位 → 跳過此 item
       ├─ 不走 source graph
       ├─ 直接以 groupKey 作為 key、prompt 作為 value
       └─ 組裝 renderedAnalysisBlock
```

### Render 行為

persona_tags renderer 的設計與 persona_notes 類似：直接 render LinkChat 傳來的內容。

```text
不走 source graph 的原因：
├─ tag 的意義說明（prompt）由使用者在 LinkChat 端定義
├─ LinkChat 把 prompt 文字直接傳給 Internal
├─ Internal 只負責組裝，不負責查表
└─ 這與 astrology 的根本差異在於語意來源：
   ├─ astrology → 語意由 Internal 的 source graph 定義
   └─ persona_tags → 語意由使用者直接提供
```

### Render 格式

```text
### [analysis:persona_tags]
role: 這個對象更適合用教學式、步驟式、降低壓力的方式互動
communication_style: 先暖身、先降低防備，再進主題
```

key 命名規則：
- 使用 `groupKey` 作為 render 的 key
- 使用 `prompt`（使用者定義的意義說明）作為 render 的 value
- Internal 原文使用 prompt，不做摘要、重寫或翻譯

### Multi-Select Render

同一個 groupKey 下若有多個 tagKey（multi-select），每個 tag 分行列出：

```text
### [analysis:persona_tags]
role: 這個對象更適合用教學式、步驟式、降低壓力的方式互動
role: 同伴式學習，喜歡一起討論
communication_style: 先暖身、先降低防備，再進主題
```

### Payload Shape

```json
{
  "analysisType": "persona_tags",
  "payload": {
    "selected": [
      {
        "groupKey": "role",
        "tagKey": "student",
        "prompt": "這個對象更適合用教學式、步驟式、降低壓力的方式互動"
      },
      {
        "groupKey": "communication_style",
        "tagKey": "slow_warmup",
        "prompt": "先暖身、先降低防備，再進主題"
      }
    ]
  }
}
```

payload 欄位說明：
- `groupKey`：tag 所屬群組的 canonical key
- `tagKey`：tag 自身的 canonical key
- `prompt`：使用者在 LinkChat 為這個 tag 定義的意義說明，即 Internal prompt 中要呈現的文字

### persona_tags 不使用 theoryVersion

persona_tags payload 不帶 `theoryVersion`。renderer 應接受 `theoryVersion=nil`，render 結果不應包含 `theory_version` 行。

### persona_tags 不使用 weighted entries

persona_tags 不使用 `weightPercent` 機制。每個 selected tag 是等權重參與 prompt 組裝。

## Prompt 邊界設計

```text
最終 prompt 分工：

  [RAW_USER_TEXT]
  └─ 只放本次問題（使用者這次真的想問的）

  [SUBJECT_PROFILE]
  ├─ astrology       ← 既有（語意由 Internal source graph 提供）
  ├─ mbti            ← 既有（直接 flatten）
  ├─ persona_notes   ← 新增（直接 flatten 使用者原文）
  └─ persona_tags    ← 新增（直接 flatten LinkChat 傳來的 prompt）

  [PROMPT_BLOCK-*]
  └─ 依 source order 排列

  [USER_INPUT]
  └─ optional
```

這樣的好處：
- 使用者本次需求與人物背景分開
- 補充三句與 tag 不會被當成 raw user instruction
- 未來若要單獨調整 notes/tag renderer，不必動 `text` 語意

## Complete Request Example

```json
{
  "appId": "linkchat",
  "builderId": 101,
  "text": "我這次該怎麼跟他談作業？",
  "subjectProfile": {
    "subjectId": "subject-001",
    "analysisPayloads": [
      {
        "analysisType": "astrology",
        "theoryVersion": "astro_v1",
        "payload": {
          "sun_sign": [{"key": "capricorn"}],
          "moon_sign": [{"key": "pisces"}],
          "rising_sign": [{"key": "gemini"}]
        }
      },
      {
        "analysisType": "persona_notes",
        "payload": {
          "lines": [
            "慢熟，剛開始不太主動聊天",
            "壓力大時會先自己消化",
            "如果先給步驟，會比較願意配合"
          ]
        }
      },
      {
        "analysisType": "persona_tags",
        "payload": {
          "selected": [
            {
              "groupKey": "role",
              "tagKey": "student",
              "prompt": "這個對象更適合用教學式、步驟式、降低壓力的方式互動"
            },
            {
              "groupKey": "communication_style",
              "tagKey": "slow_warmup",
              "prompt": "先暖身、先降低防備，再進主題"
            }
          ]
        }
      }
    ]
  }
}
```

## 與既有 Renderer 的對照

| 面向 | Astrology | MBTI | Persona Notes | Persona Tags |
|------|-----------|------|---------------|--------------|
| render 模式 | source graph lookup | 直接 flatten | 直接 flatten | 直接 flatten |
| 語意來源 | Internal fragment source | payload 原值 | 使用者原文 | 使用者定義的 prompt |
| 需要 Firestore source data | ✅ 需要 | ❌ 不需要 | ❌ 不需要 | ❌ 不需要 |
| weighted entries | ✅ 支援 | ❌ 不使用 | ❌ 不使用 | ❌ 不使用 |
| theoryVersion | ✅ 可帶 | ✅ 可帶 | ❌ 不使用 | ❌ 不使用 |
| Internal 端驗證 | 無額外驗證 | 無額外驗證 | max 3 條 | 無額外驗證 |

## Error Strategy

### 新增錯誤碼

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|----------|
| PERSONA_NOTES_LIMIT_EXCEEDED | 400 | persona_notes 的 lines 超過 3 條（trim 後非空的條數） |

### 沿用的既有錯誤碼

| 錯誤碼 | 觸發條件 |
|--------|----------|
| UNSUPPORTED_ANALYSIS_TYPE | linkChatAnalysisRenderer 收到未知 analysisType |
| INVALID_SOURCE_MODULE_KEY | source 的 moduleKey 格式非法 |

## Implementation Notes

### personaNotesLinkChatAnalysisRenderer

```text
實作要點：
├─ 讀 payload["lines"]
│  ├─ 型別是 []any → 逐項 cast string
│  ├─ 跳過非 string 項
│  └─ trim 每條 → 跳過空字串
├─ 剩餘超過 3 條 → PERSONA_NOTES_LIMIT_EXCEEDED
├─ 不走 source graph
├─ 不走 weighted entries
└─ 組 renderedAnalysisBlock
    └─ Lines = []renderedProfileLine{
         {Key: "note_1", Values: []string{line1}},
         {Key: "note_2", Values: []string{line2}},
         {Key: "note_3", Values: []string{line3}},
       }
```

### personaTagsLinkChatAnalysisRenderer

```text
實作要點：
├─ 讀 payload["selected"]
│  ├─ 型別是 []any → 逐項 cast map[string]any
│  ├─ 從每個 map 讀 "groupKey"、"tagKey"、"prompt"
│  └─ 缺任一必填欄位 → 跳過此 item
├─ 不走 source graph
├─ 不走 weighted entries
├─ 直接以 groupKey 作為 key、prompt 作為 value
├─ 同一 groupKey 多個 tag → 分行列出
└─ 組 renderedAnalysisBlock
    └─ Lines = []renderedProfileLine{
         {Key: "role", Values: []string{prompt1}},
         {Key: "communication_style", Values: []string{prompt2}},
       }
```

## Boundary Notes

- LinkChat 決定本次送來哪些 analysis payloads
- LinkChat 保存 tag catalog、tag 意義說明（prompt）、人物補充三句原文
- Internal 不保存 LinkChat 的人物原始資料
- Internal 不負責查表映射 tag → prompt（由 LinkChat 直接傳入）
- Internal 以 prompt 原文直接組裝，不做語意轉換
- persona_notes / persona_tags 只是新的 analysisType，不改變架構

## 與 LinkChat SDD 的契約對齊

LinkChat SDD 中 persona_tags 的 payload shape 需要更新：

```text
原始 LinkChat SDD payload：
  {"selected": [{"groupKey": "role", "tagKey": "student"}]}

更新後 payload：
  {"selected": [{"groupKey": "role", "tagKey": "student", "prompt": "教學式互動..."}]}

原因：
├─ 使用者在 LinkChat 端為 tag 定義意義說明
├─ LinkChat 把 prompt 文字傳給 Internal
└─ Internal 直接組裝，不做 source graph lookup
```

## Version Notes

### 第一版

- persona_notes renderer：直接 flatten render lines
- persona_tags renderer：直接 flatten render LinkChat 傳來的 prompt
- Internal 端驗證 persona_notes max 3 條
- 不需要新增 Firestore source data
- 不需要新增 source graph

### 未來可延伸

- persona_notes 條數上限可調整
- persona_tags 新增 tag group 只需 LinkChat 端配置，Internal 不需改 code
- persona_tags 若未來需要 Internal 端做額外語意豐富，可升級為 source graph 模式
