# Gatekeeper Module Spec

## Purpose
這份文件是 gatekeeper module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Gatekeeper 是系統的 consult validation boundary。HTTP handler 直接承接 public / external HTTP routes；gRPC transport 則由 grpcapi adapter 承接後再呼叫同一套 gatekeeper usecase。

它負責接收 consult/builders 請求、做基礎驗證、解析 client IP，然後把請求交給 Builder。

對應 Java：`com.citrus.internalaicopilot.gatekeeper`

## Layering In This Module

```text
Handler -> UseCase -> Service
```

此模組通常不直接持有 repository，因為主要工作是：
- request parsing
- guard validation
- 呼叫 builder

gRPC transport adapter 不在此 module 內，但應重用同一個 gatekeeper usecase/service。

## Responsibilities
- 回傳 active builders 給前端下拉選單
- 回傳 external app 可使用的 active builders 給外部整合系統
- 接收 `multipart/form-data` generic consult 請求
- 接收 external app 的 `multipart/form-data` generic consult 請求
- 接收 grpcapi 轉進來的 generic `Consult` command
- 接收 grpcapi 轉進來的 `ProfileConsult` command
- 驗證 `appId`
- 驗證 `builderId`
- 驗證 `outputFormat`
- 驗證檔案數量、單檔大小、總大小與副檔名
- 驗證 `appId -> builderId` 授權
- 驗證 structured `subjectProfile` 的共享 envelope 與 `analysisType`
- 為 builder command 設定正確的 `ConsultMode`
- 解析 client IP
- 將 validated `appId` 或 optional public `appId` 傳給 builder，供 prompt strategy 選擇
- 將合法請求轉交給 builder usecase

## Layer Responsibilities

### Handler
- parse HTTP request
- parse multipart files
- call gatekeeper usecase
- write `ApiResponse`

### UseCase
- orchestration for `ListBuilders` / `ListExternalBuilders` / `Consult` / `ExternalConsult`
- bridge gatekeeper service and builder usecase
- map validated request to builder command
- 承接 HTTP 或 gRPC transport 已轉好的 consult payload
- 保留 generic / profile consult 的明確 mode 語意

### Service
- guard validation
- client IP resolution
- structured profile consult field validation

## Request Contract

### `GET /api/builders`
回傳 active builders，依 `builderId ASC` 排序。

每筆資料包含：
- `builderId`
- `builderCode`
- `groupKey`
- `groupLabel`
- `name`
- `description`
- `includeFile`
- `defaultOutputFormat`

### `GET /api/external/builders`
Header：
- `X-App-Id` required

回傳此 external app 可使用的 active builders，依 `builderId ASC` 排序。

### `POST /api/external/consult`
`Content-Type: multipart/form-data`

Header：
- `X-App-Id` required

欄位：
- `builderId` required
- `text` optional
- `outputFormat` optional
- `files` optional, multiple
- `appId` optional, only for public/local-dev prompt-strategy testing

### `POST /api/consult`
`Content-Type: multipart/form-data`

欄位：
- `builderId` required
- `text` optional
- `outputFormat` optional
- `files` optional, multiple

支援副檔名：
- document: `pdf`, `doc`, `docx`
- image: `jpg`, `jpeg`, `png`, `webp`, `gif`, `bmp`

### `POST /api/profile-consult`
`Content-Type: application/json`

欄位：
- `appId` optional, only for public/local-dev prompt-strategy testing
- `builderId` required
- `subjectProfile` optional
- `text` optional

限制：
- 此 route 僅供 local/dev prompt testing 使用
- 不承擔 external app auth 語意
- production 不應直接對公網暴露此 route

### gRPC generic `Consult`
generic `Consult` 仍承接 generic consult 語意：
- `appId`
- `builderId`
- `text`
- `outputFormat`
- `attachments`
- `clientIp`

generic `Consult` 最終應映射為 `ConsultModeGeneric`。

### gRPC `ProfileConsult`
`ProfileConsult` 對 LinkChat profile-analysis 這條線應至少承載：
- `appId`
- `builderId`
- `subjectProfile` optional
- `text` optional
- `clientIp` optional

其中：
- `subjectProfile` 代表 external app 已正規化好的 subject 資料
- external app 應只送本次真的有資料的 analysis payload，不再額外傳 top-level module list
- `subjectProfile` 可帶 `analysis payloads[]`，每個 payload 需有 stable `analysisType`
- `subjectProfile` 缺值且 `text!=""` 是合法的 text-only profile request
- 若某個 analysis type 需要 Internal-side code mapping，對應 payload 應帶 `theoryVersion`

`ProfileConsult` 最終應映射為 `ConsultModeProfile`。

## Validation Rules

```text
Request 進入 Gatekeeper
     │
     ▼
┌─ App 驗證（僅 external HTTP / app-scoped gRPC）──────────────┐
│  appId 存在？ ─── 否 → APP_ID_MISSING                       │
│       │ 是                                                    │
│       ▼                                                       │
│  app 存在？ ─── 否 → APP_NOT_FOUND                           │
│       │ 是                                                    │
│       ▼                                                       │
│  app active? ─── 否 → APP_INACTIVE                           │
│       │ 是                                                    │
│       ▼                                                       │
│  builderId 在 app 授權名單內？ ─── 否 → APP_BUILDER_FORBIDDEN│
│       │ 是                                                    │
│       ▼                                                       │
│  （通過 app 驗證）                                            │
│                                                               │
│  ※ public POST /api/consult 的 appId 為 optional，           │
│    僅作 prompt strategy hint，不走此 app auth 流程            │
└───────────────────────────────────┬───────────────────────────┘
                                    ▼
┌─ Builder 驗證 ────────────────────────────────────────────────┐
│  builderId 有值？ ─── 否 → BUILDER_ID_MISSING                │
│       │ 是                                                    │
│       ▼                                                       │
│  builder 存在？ ─── 否 → BUILDER_NOT_FOUND                   │
│       │ 是                                                    │
│       ▼                                                       │
│  builder active? ─── 否 → BUILDER_INACTIVE                   │
│       │ 是                                                    │
│       ▼                                                       │
│  （通過 builder 驗證）                                        │
└───────────────────────────────────┬───────────────────────────┘
                                    ▼
┌─ 格式與附件驗證 ──────────────────────────────────────────────┐
│  outputFormat 有值？                                          │
│   ├── 有 → 是 markdown 或 xlsx？                             │
│   │         ├── 否 → UNSUPPORTED_OUTPUT_FORMAT               │
│   │         └── 是 → 通過                                    │
│   └── 無 → 通過                                              │
│                                                               │
│  client IP 可解析？ ─── 否 → CLIENT_IP_MISSING               │
│                                                               │
│  附件驗證：                                                   │
│   ├── 檔案數超過限制？ → FILE_COUNT_EXCEEDED                 │
│   ├── 單檔大小超過限制？ → FILE_SIZE_EXCEEDED                │
│   └── 總大小超過限制？ → FILE_TOTAL_SIZE_EXCEEDED            │
└───────────────────────────────────┬───────────────────────────┘
                                    ▼
┌─ Profile Consult 驗證（僅 ConsultModeProfile）────────────────┐
│                                                               │
│  subjectProfile 驗證：                                        │
│   ├── 無值 → 允許（text-only profile request）               │
│   └── 有值 ↓                                                 │
│        ├── subjectId 必填                                     │
│        ├── analysis payload 不可重複（同 analysisType）       │
│        └── 逐一檢查每個 analysis payload：                    │
│              ├── analysisType 不可為空白                      │
│              ├── analysisType 須符合 stable key 格式          │
│              └── 不在 gatekeeper 內解析 astrology/mbti 細節   │
│                                                               │
│  theoryVersion 驗證：                                         │
│   ├── 若提供，不可為空白                                      │
│   ├── linkchat + astrology → 必填                             │
│   └── linkchat + mbti → 可省略                                │
└───────────────────────────────────┬───────────────────────────┘
                                    ▼
                            全部通過，轉交 Builder
```

## Notes
- Gatekeeper 不做附件內容解析
- Gatekeeper 不做附件落地保存
- `appId` 目前作為 external app 的業務授權 key；Cloud Run service-to-service auth 將於部署階段補上
- 尚未實作 IP allowlist / blocklist
- 尚未實作 MIME validation
- Gatekeeper 不負責 LinkChat 的 module entitlement 與缺資料剔除；那是 external app 的本地 gatekeeping
- Gatekeeper 不負責 analysis-type-specific payload parsing；那是 builder 內 LinkChat 第二層 factory 的責任
- Gatekeeper 不負責 raw theory value -> Internal private code mapping；那是 builder prompt strategy 與 codebook repository 的責任
- public prompt-testing routes 的安全性預設由部署/環境隔離保護，不由 gatekeeper 在第一版內做 app auth
