# RAG Module Spec

## Purpose
這份文件是 rag module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
RAG 是統一的資料補充與 retrieval module。builder 不需要知道 RAG 是靜態內容、向量檢索，還是外部資料；builder 只傳入已選入的 `ragConfigs`，由 rag module 負責 resolve。

對應 Java：
- `com.citrus.internalaicopilot.rag`

同時也承接 Java `retrieval_mode` 的擴充意圖。

## Layering In This Module

```text
UseCase -> Service -> Repository
```

### Typical split
- UseCase：對 builder 暴露 resolver 入口
- Service：依 `retrievalMode` 做 resolve / normalize / fallback
- Repository：未來向量或外部來源的 backing store

## Responsibilities
- 提供統一 resolver 給 builder
- 根據 `retrievalMode` 決定如何取得內容
- 回傳可供 builder 組裝的 resolved RAG content
- 保留未來 vector / external retrieval 擴充能力

## Important Rule
不要把 RAG 分成兩套模型。

原則：
- graph 裡存的是 `ragConfig`
- resolve 方式由 `retrievalMode` 決定
- builder 不區分靜態與動態

## Resolve Baseline
第一版至少需要支援：
- `full_context`

未來保留：
- `vector_search`
- `external_api`

## Boundary
- rag module 負責 resolve content
- builder module 負責最終 prompt 組裝與順序
- override 最終是否套用由 builder 決定
- LinkChat profile-analysis 的 `analysisModules` 與 source `moduleKey` 選擇屬於 builder，不屬於 rag
