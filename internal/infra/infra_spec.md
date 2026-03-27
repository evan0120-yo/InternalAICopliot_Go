# Infra Module Spec

## Purpose
這份文件是 infra module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Infra 提供跨模組共用的基礎設施。包含應用設定、Firestore 實作、API response envelope、錯誤處理、啟動 wiring 與開發用 bootstrap。

在 LinkChat profile-analysis 這條線上，infra 也負責承接 module-aware builder graph 的資料欄位定義，例如 source 的 optional `moduleKey`，以及 composable source graph 所需的 `sourceType` / `matchKey` / `sourceIds[]`。

對應 Java：
- `com.citrus.internalaicopilot.common`
- `com.citrus.internalaicopilot.initData`

## Responsibilities
- config
- Firestore client / repository implementations
- `ApiResponse`
- business error types
- app bootstrap / dependency wiring
- dev seed / bootstrap
- app prompt config persistence / cache

## What Infra Should Not Do
- 不承擔 builder / rag / output 的業務規則
- 不在 infra 內寫 prompt assembly
- 不在 infra 內寫 retrieval 判斷
- 不在 infra 內做 module entitlement 決策

## Firestore Scope
infra 只承接 Firestore 的實作與 wiring，不替 domain 決定業務語意。

目前基準 collection：
- `apps/{appId}`
- `builders/{builderId}`
- `builders/{builderId}/sources/{sourceId}`
- `builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}`
- `templates/{templateId}`
- `templates/{templateId}/templateRags/{templateRagId}`
- `appPromptConfigs/{appId}`

未來可能由 rag module 使用的 backing store：
- `rag_sources/{ragSourceId}`
- `rag_vectors/{vectorId}`

## Config
目前設定來源以環境變數為主，但具體部署平台與 router/library 仍屬待定，不應在 infra 文件中視為已定案。

## Local Bootstrap
infra 需要承接 local 開發用的 reset/seed bootstrap 能力。

其行為目標應與 Java `initData + create-drop` 對齊：
- 啟動 local app 前可先清除既有開發資料
- 之後重新載入固定 seed data
- 讓 frontend、API 與整合測試共用可預期的初始狀態

目前正式實作：
- Firestore emulator：清空開發用 collections/documents 後再 seed
- collection baseline 仍是：
  - `apps/{appId}`
  - `builders/{builderId}`
  - `builders/{builderId}/sources/{sourceId}`
  - `builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}`
  - `templates/{templateId}`
  - `templates/{templateId}/templateRags/{templateRagId}`

限制：
- reset 動作必須受 local/dev config 保護
- infra 可以提供 bootstrap helper，但不應把 builder/rag/output 的業務規則搬進 infra

## Response Contract
所有 HTTP handler 應回統一 envelope：

```json
{
  "success": true,
  "data": {},
  "error": null
}
```

## Error Contract
業務錯誤應包含：
- code
- message
- http status

並由統一 error handling 轉為 response。

## Module-Aware Builder Graph Fields
infra 應承接 source document 的 composable graph 欄位。

source 應至少支援：
- `moduleKey` optional
- `sourceType` optional
- `matchKey` optional
- `sourceIds[]` optional
- `tags[]` optional

規則：
- `moduleKey` 缺值或空值，表示此 source 永遠可用
- `common` 是保留語意，write path 應正規化為缺值 / 空值
- `sourceType=primary`
  - 表示此 source 會參與 builder 的主組裝流程
  - `orderNo` 仍有效
- `sourceType=fragment`
  - 表示此 source 不直接作為頂層 prompt block，而是供其他 source 組合引用
  - admin UI 應能與 primary sources 分群顯示
- `matchKey`
  - 供 app-specific strategy 直接以 canonical key 解析到某個 fragment source
  - 應視為 builder 內穩定 lookup key，而不是 UI label
- `sourceIds[]`
  - 表示此 source 會再展開哪些 child sources
  - 陣列順序即展開順序，infra 只需原樣保存，不應再自行排序
- `tags[]`
  - 只作 admin UI / 人工維護時的搜尋、分群、過濾輔助
  - 不應作為 runtime prompt assembly 的 lookup key
  - strategy / builder 不應依賴 tags 決定 source traversal
  - 建議保存 canonical tag strings，不保存 UI 專用的 `#` 前綴
- rag ownership 不改變
  - rag 仍屬於自己的 source
  - 不因 composable source graph 需求改成 `source -> rag pool`
- infra 只負責存取與映射，不負責 module selection、graph traversal、去重或防循環

## App Prompt Config And Canonical Key Ownership
infra 應持有 app-aware prompt strategy 所需的資料 source of truth。

`appPromptConfigs/{appId}` 應至少能表達：
- `appId`
- `strategyKey`
- `active`

規則：
- builder / strategy 應透過 repository 讀取 app prompt config 與 source graph，不應把 app-specific lookup key 寫死在 prompt strategy 內。
- LinkChat 這條線的 raw value / alias 正規化責任留在 external app 自己的 DB / backend。
- Internal Firestore 不再需要獨立的 `theoryMappings` collection。
- strategy 應直接以 external app 傳來的 canonical value 對 `source.matchKey` 做 lookup。
- infra 只負責資料存取、seed 與 cache wiring，不負責決定 prompt 如何 render。
- 第一版 cache 不要求 TTL 或主動 invalidation；若 Firestore 中的策略設定或 source graph 被修改，需重新啟動服務才保證吃到最新值。
- 可編輯的 prompt 內容應落在 source / rag graph。
- slot-level 語意標籤（例如 `人生主軸`、`情緒本能`）應放在 primary source 的 `prompts`。
