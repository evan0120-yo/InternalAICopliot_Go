# Gatekeeper Module Spec

## Purpose
這份文件是 gatekeeper module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Gatekeeper 是系統的 consult validation boundary。HTTP handler 直接承接 public / external HTTP routes；gRPC transport 則由 grpcapi adapter 承接後再呼叫同一套 gatekeeper usecase。

它負責接收 consult/builders 請求、做基礎驗證、解析 client IP，然後把請求交給 Builder。

在第一版 promptguard integration 中，Gatekeeper 的 `ProfileConsult` / `PublicProfileConsult` orchestration 會在通過既有驗證後，先把 `raw user text` 送進 promptguard usecase；只有 promptguard 明確放行後，才會繼續進 builder 主 consult flow。

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
- 在第一版 astrology/profile 主流程中，於 builder consult 前先呼叫 promptguard usecase
- 依 promptguard 結果決定直接回 blocked business response，或繼續轉交 builder usecase
- 將合法請求轉交給 builder usecase

## Layer Responsibilities

### Handler
- parse HTTP request
- parse multipart files
- call gatekeeper usecase
- write `ApiResponse`

### UseCase
- orchestration for `ListBuilders` / `ListExternalBuilders` / `Consult` / `ExternalConsult`
- bridge gatekeeper service、promptguard usecase 與 builder usecase
- map validated request to builder command
- 承接 HTTP 或 gRPC transport 已轉好的 consult payload
- 保留 generic / profile consult 的明確 mode 語意
- 對第一版 profile astrology flow 負責 `validate -> promptguard -> builder consult` 的串接順序
- gatekeeper usecase 應只依 promptguard evaluation 做放行或擋下決策，不自行組 guard prompt 或解析 guard JSON

### Service
- guard validation
- client IP resolution
- structured profile consult field validation
- gatekeeper service 仍只負責同步驗證，不直接承擔 promptguard orchestration

## Profile PromptGuard Integration

第一版 promptguard integration 只接在星座/profile 主流程，不改 generic consult 路徑。

```text
ProfileConsult / PublicProfileConsult
        │
        ├─ Gatekeeper Service validation
        │
        ├─ raw text 為空？
        │   ├─ 是 -> 直接進 builder consult 主流程
        │   └─ 否
        │
        ├─ promptguard usecase
        │   ├─ allow -> 繼續 builder consult 主流程
        │   ├─ block -> 直接回 blocked business response
        │   └─ system/internal failure -> 回系統錯誤
        │
        └─ builder consult 主流程
```

規則：
- Gatekeeper 只把 `raw user text` 與最小必要 consult context 交給 promptguard usecase。
- 若 request 沒有 `raw user text`，Gatekeeper 不應為了 promptguard 額外組 guard prompt。
- 第一版 current scope 只在 `builderCode=linkchat-astrology` 時啟用 promptguard。
- Gatekeeper 不應自己接 builder 來組 guard prompt，也不應自己打 aiclient 做第二層 guard。
- `status=false` 的 promptguard block 應視為正常 business response，而不是 validation 4xx。
- 第一版不把 generic consult / external generic consult 納入 promptguard integration。

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
- `mode` optional
  - `preview_full`
  - `preview_prompt_body_only`
  - `live`

限制：
- 此 route 僅供 local/dev prompt testing 使用
- 不承擔 external app auth 語意
- production 不應直接對公網暴露此 route
- gatekeeper 只驗 `mode` 是否為支援值，實際輸出策略由下游 aiclient 決定
- 第一版 promptguard integration 應先套用在這條 profile consult 星座主流程

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
- external app 若有自己的理論版本標記，可帶 `theoryVersion`；Internal canonical-key path 不以此作為必填欄位
- 若某個 analysis type 採 weighted canonical entry 形狀，`payload.<slotKey>` 可為：
  - `["capricorn"]`
  - `[{ "key": "capricorn", "weightPercent": 70 }]`

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
│              ├── composable weighted entry envelope 可驗證：   │
│              │     ├── `key` 必填                             │
│              │     ├── 單一 entry 可省略 `weightPercent`      │
│              │     ├── 多 entry 時每筆都需有 `weightPercent`  │
│              │     └── 多 entry 的百分比總和需為 100          │
│              └── 不在 gatekeeper 內解析 astrology/mbti 語意   │
│                                                               │
│  theoryVersion 驗證：                                         │
│   └── 若提供，不可為空白                                      │
└───────────────────────────────────┬───────────────────────────┘
                                    ▼
┌─ PromptGuard（第一版僅 Profile astrology flow）───────────────┐
│  raw user text 為空？                                         │
│   ├── 是 -> 跳過 promptguard                                  │
│   └── 否                                                      │
│        ├── promptguard allow -> 繼續                          │
│        ├── promptguard block -> 回 blocked business response  │
│        └── promptguard system failure -> 回系統錯誤           │
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
- Gatekeeper 不負責 raw value / alias -> canonical key 正規化；那是 external app（例如 LinkChat）自己的責任
- Gatekeeper 可驗共享的 weighted-entry envelope 規則，但不負責解讀 `capricorn`、`pisces` 等 domain key 的語意
- public prompt-testing routes 的安全性預設由部署/環境隔離保護，不由 gatekeeper 在第一版內做 app auth
- promptguard 是獨立 module；gatekeeper 只在 usecase 層調用它，不把 promptguard 寫進 gatekeeper service
- Gatekeeper 不為 promptguard path 讀取 source / rag；那是下游 promptguard + builder 的邊界
