# GRPC API Module Spec

## Purpose
這份文件是 grpcapi module 的規格文件，用來定義 Internal gRPC transport adapter 的責任、邊界，以及 generic `Consult` 與 `ProfileConsult` 兩條 integration contract。

## Overview
grpcapi 是 Internal 對 external integrations 暴露的 gRPC transport adapter。它負責把 gRPC request 轉成 gatekeeper usecase 可理解的 command，並將 business error 映射為 gRPC status。

對 LinkChat profile-analysis 這條線來說，grpcapi 承接的是 `ProfileConsult` structured request，而不是 LinkChat 本地的開通規則或對象資料查詢。

## Layering

```text
gRPC Service -> gatekeeper UseCase
```

`grpcapi` 不是新的 domain module；它只是 transport adapter。

## Responsibilities
- 暴露 `IntegrationService`
- 保留 `ListBuilders` 作為 discovery / bootstrap API
- 將 gRPC request 轉成 gatekeeper usecase command
- 為 command 設定明確的 `ConsultMode`
- 將 attachments 與 client IP 做 transport-level adaptation
- 為 business error 設定正確的 gRPC status code 與 `ErrorInfo.reason`
- 保留 generic `Consult`
- 承接 `ProfileConsult`
- 將 `app_id` 原樣往下傳，供 gatekeeper 做授權並供 builder 選 prompt strategy
- 將 structured profile envelope 原樣往下傳，供 builder 內部的 LinkChat analysis factory 解讀
- 將 `theory_version` 原樣往下傳，供 builder strategy / codebook 使用

## What grpcapi Must Not Do
- 不負責 LinkChat 對象資料查詢
- 不負責 module entitlement 判斷
- 不負責 analysis-type 專屬 payload parsing
- 不負責 prompt selection
- 不直接存取 builder/source/rag repository
- 不靠 structured profile payload 是否為空來猜測 consult mode

## Generic Consult Contract
現有 `Consult` 保留 generic consult 語意，主要承接：
- `app_id`
- `builder_id`
- `text`
- `output_format`
- `attachments`
- `client_ip`

generic `Consult` 對應 `ConsultModeGeneric`。

## ProfileConsult Contract
`ProfileConsult` 對 LinkChat profile-analysis 這條線應至少承載：
- `app_id`
- `builder_id`
- `subject_profile` optional
- `text` optional
- `client_ip` optional

`subject_profile` 應至少包含：
- `subject_id`
- `analysis payloads[]`

每個 analysis payload 應至少包含：
- `analysis_type`
- `theory_version` optional, required when the analysis type uses Internal-side code mapping
- analysis-type-specific `payload`

`ProfileConsult` 對應 `ConsultModeProfile`。

## ProfileConsult Mode Notes
- `ConsultModeProfile` 必須由 RPC path 決定，不可由 `subject_profile` 或其 analysis payloads 是否為空推斷。
- `subject_profile` 缺值且 `text!=""` 是合法的 text-only profile request。
- text-only profile request 仍必須維持 `ConsultModeProfile`，讓 builder 只跑 common sources。

## Discovery Rule
`ListBuilders` 仍保留為 integration discovery surface，但 LinkChat profile-analysis hot path 不應每次 consult 前都先叫一次 `ListBuilders`。

## Validation Split

```text
驗證職責三層分配

┌─────────────────────────────────────────────────────────────┐
│                     LinkChat（外部系統）                     │
│                                                             │
│  ├── 模組開通（module entitlement）                         │
│  ├── 資料完整性檢查                                         │
│  ├── analysis payload 剔除（缺資料時不送）                  │
│  └── payload normalization                                  │
└─────────────────────────┬───────────────────────────────────┘
                          ↓ gRPC request
┌─────────────────────────────────────────────────────────────┐
│                  grpcapi（Transport Adapter）                │
│                                                             │
│  ├── transport shape 驗證                                   │
│  ├── 設定 explicit ConsultMode（由 RPC path 決定）          │
│  ├── client IP fallback                                     │
│  ├── business error → gRPC status mapping                   │
│  └── app_id / optional theory_version / structured envelope │
│      無損轉交                                               │
└─────────────────────────┬───────────────────────────────────┘
                          ↓ gatekeeper command
┌─────────────────────────────────────────────────────────────┐
│                 gatekeeper（Business Guard）                 │
│                                                             │
│  ├── appId 授權驗證                                         │
│  ├── builderId 存在性與狀態驗證                              │
│  ├── output format 驗證                                     │
│  ├── attachment 驗證（數量 / 大小 / 副檔名）                │
│  └── structured profile consult 驗證                        │
│       （subjectProfile envelope / analysisType / optional   │
│        theoryVersion metadata）                             │
└─────────────────────────┬───────────────────────────────────┘
                          ↓ validated command
┌─────────────────────────────────────────────────────────────┐
│                    Builder UseCase                          │
│                                                             │
│  ├── 第一層 strategy：default / linkchat                    │
│  └── LinkChat 第二層 factory：依 analysis_type 分流         │
└─────────────────────────────────────────────────────────────┘
```

## Output Notes
- generic `Consult` 仍保留 generic file payload 能力。
- `ProfileConsult` 第一版只要求純文字回應。
- `ProfileConsult` 這條線預設應走 `includeFile=false` 的 builder。
- `app_id` 除了 external auth 外，也會影響 downstream prompt strategy selection；grpcapi 不直接解讀該業務語意。
- `builder_id` 仍保留在 contract 內，作為整體 builder/source/rag 骨架選擇鍵。
