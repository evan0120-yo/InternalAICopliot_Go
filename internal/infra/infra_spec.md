# Infra Module Spec

## Purpose
這份文件是 infra module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Infra 提供跨模組共用的基礎設施。包含應用設定、Firestore 實作、API response envelope、錯誤處理、啟動 wiring 與開發用 bootstrap。

在 LinkChat profile-analysis 這條線上，infra 也負責承接 module-aware builder graph 的資料欄位定義，例如 source 的 optional `moduleKey`。

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
- app prompt config 與 theory mapping table persistence / cache

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
- `theoryMappings/{mappingId}`

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
infra 應承接 source document 的 optional `moduleKey`。

規則：
- `moduleKey` 缺值或空值，表示此 source 永遠可用
- `common` 是保留語意，write path 應正規化為缺值 / 空值
- `moduleKey` 有值，表示 builder 需依 request `analysisModules` 決定是否載入
- infra 只負責存取與映射，不負責 module selection

## App Prompt Config And Theory Mapping
infra 應持有 app-aware prompt strategy 所需的資料 source of truth。

`appPromptConfigs/{appId}` 應至少能表達：
- `appId`
- `strategyKey`
- `active`

`theoryMappings/{mappingId}` 應至少能表達：
- `appId`
- `moduleKey`
- `theoryVersion`
- `mappingType` (`slot` / `value`)
- `slotKey`
- `rawValue` optional
- `semanticPrompt`
- `active`

規則：
- builder / strategy 應透過 repository 讀取這些資料，不應把 mapping table 寫死在 prompt strategy 內。
- runtime 可自行快取 `appId + moduleKey + theoryVersion` 範圍內的 mapping，但 Firestore 仍是 source of truth。
- infra 只負責資料存取、seed 與 cache wiring，不負責決定 prompt 如何 render。
- 第一版 cache 不要求 TTL 或主動 invalidation；若 Firestore 中的策略設定或 mapping 被修改，需重新啟動服務才保證吃到最新值。
- theoryMappings 的目標是支援 Internal 直接翻譯出最終語意片段，而不是把 Internal private code 暴露給 AI。
