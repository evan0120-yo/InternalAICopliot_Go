# GRPC API Module Spec

## Purpose
這份文件是 grpcapi module 的規格文件，用來定義 Internal gRPC transport adapter 的責任、邊界，以及 generic `Consult`、`ProfileConsult` 與 task-specific extraction contract。

## Overview
grpcapi 是 Internal 對 external integrations 暴露的 gRPC transport adapter。它負責把 gRPC request 轉成 gatekeeper usecase 可理解的 command，並將 business error 映射為 gRPC status。

對 LinkChat profile-analysis 這條線來說，grpcapi 承接的是 `ProfileConsult` structured request，而不是 LinkChat 本地的開通規則或對象資料查詢。

同一個 grpcapi / `IntegrationService` 可以同時承接多種 external contract，例如：
- LinkChat profile-analysis
- 私人 LineBot 備忘錄 / CRUD 抽取
- future app-specific AI tasks

## Layering

```text
gRPC Service -> gatekeeper UseCase
```

`grpcapi` 不是新的 domain module；它只是 transport adapter。

## Responsibilities
- 暴露 `IntegrationService`
- 保留 `ListBuilders` 作為 discovery / bootstrap API
- 將 gRPC request 轉成 gatekeeper usecase command
- 允許不同 external system 使用不同 RPC contract
- 為 command 設定明確的 `ConsultMode`
- 將 attachments 與 client IP 做 transport-level adaptation
- 為 business error 設定正確的 gRPC status code 與 `ErrorInfo.reason`
- 保留 generic `Consult`
- 承接 `ProfileConsult`
- 將 `app_id` 原樣往下傳，供 gatekeeper 做授權並供 builder 選 prompt strategy
- 將 structured profile envelope 原樣往下傳，供 builder 內部的 LinkChat analysis factory 解讀
- 將 `theory_version` 原樣往下傳，供 builder strategy / codebook 使用

## Multi-Contract Rule
grpcapi 可以在同一個 service 內暴露多個不同的 external contract。

```text
IntegrationService
├─ Consult
├─ ProfileConsult
└─ future RPCs
   ├─ LineTaskConsult
   └─ other task-specific contracts
```

規則：
- 不同 external system 的欄位 shape 可以不同。
- grpcapi 的責任是把不同 request shape 轉成 Internal command。
- 不需要為了新的外部任務另外複製一份 Internal 專案或另開一個新的 gRPC server。
- transport contract 的差異應留在 grpcapi adapter，不應擴散成整個 Internal 的重複 codebase。

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
- `user_text` optional
- `intent_text` optional
- `text` optional
  - legacy alias
- `client_ip` optional

`subject_profile` 應至少包含：
- `subject_id`
- `analysis payloads[]`

每個 analysis payload 應至少包含：
- `analysis_type`
- `theory_version` optional metadata
- analysis-type-specific `payload`

若 analysis-type-specific `payload` 內含 weighted canonical entries，grpcapi 應原樣保留物件陣列形狀，例如：

```json
{
  "sun_sign": [
    { "key": "capricorn", "weightPercent": 70 },
    { "key": "aquarius", "weightPercent": 30 }
  ]
}
```

`ProfileConsult` 對應 `ConsultModeProfile`。

## ProfileConsult Mode Notes
- `ConsultModeProfile` 必須由 RPC path 決定，不可由 `subject_profile` 或其 analysis payloads 是否為空推斷。
- `subject_profile` 缺值且 `user_text!=""` 或 `intent_text!=""` 都是合法的 profile request。
- `text` 在相容期內可暫時視為 `user_text` alias。
- `user_text`-only / `intent_text`-only profile request 仍必須維持 `ConsultModeProfile`，讓 builder 只跑 common sources。

## LineTaskConsult Contract
`LineTaskConsult` 應作為 LineBot extraction 的專用 gRPC contract，不重用 `ProfileConsult`。

```text
LineTaskConsultRequest
├─ app_id
├─ builder_id
├─ message_text
├─ reference_time
├─ time_zone
└─ client_ip optional
```

```text
LineTaskConsultResponse
├─ operation
├─ summary
├─ start_at
├─ end_at
├─ location
└─ missing_fields[]
```

規則：
- `message_text` 應是 LineBot server 已去掉 `AI:` 前綴後的自然語言內容。
- `reference_time` 與 `time_zone` 是讓下游 AI 將 `明天 / 下午三點` 轉成絕對時間所必需的欄位。
- `LineTaskConsult` 對應 `ConsultModeExtract`。
- `LineTaskConsult` 的 response 應是 typed protobuf contract，不應只回 raw JSON string。
- grpcapi 不解析 AI JSON；它只接收下游已驗證的 extraction result，再映射成 protobuf response。

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
│      （包含 canonical key 與 weighted entry shape）         │
└─────────────────────────┬───────────────────────────────────┘
                          ↓ gRPC request
┌─────────────────────────────────────────────────────────────┐
│                  grpcapi（Transport Adapter）                │
│                                                             │
│  ├── transport shape 驗證                                   │
│  ├── 設定 explicit ConsultMode（由 RPC path 決定）          │
│  ├── 保留 weighted entry object arrays                      │
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
