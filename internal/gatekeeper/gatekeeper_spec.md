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
- 驗證 `analysisModules` 與 structured `subjectProfile` 的基本形狀
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
- `analysisModules[]` required
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
- `analysisModules[]`
- `subjectProfile` optional
- `text` optional
- `clientIp` optional

其中：
- `analysisModules` 代表本次實際參與分析的 modules
- `subjectProfile` 代表 external app 已正規化好的 subject 資料
- `analysisModules=[] && text!=""` 是合法的 text-only profile request
- 若某個 module 需要 Internal-side code mapping，對應 payload 應帶 `theoryVersion`

`ProfileConsult` 最終應映射為 `ConsultModeProfile`。

## Validation Rules
- `appId` 在 external HTTP 與 app-scoped gRPC integration request 為必填
- public HTTP `POST /api/consult` 的 `appId` 為 optional，且只作為 prompt strategy hint，不承擔 external app 授權語意
- external app 必須存在
- external app 必須為 `active=true`
- external app 只能取得自己被授權的 builders
- external consult / profile consult 的 `builderId` 必須在 app 授權名單內
- `builderId` 必填
- builder 必須存在
- builder 必須為 `active=true`
- `outputFormat` 若提供，必須是 `markdown` 或 `xlsx`
- client IP 必須可解析
- 檔案數不可超過設定值
- 單檔大小不可超過設定值
- 總大小不可超過設定值
- `analysisModules` 在 profile mode 可為空，但不可為 `nil` 推斷 generic mode
- `analysisModules` 應視為本次 request 的顯式 module set
- `analysisModules` 必須符合 `trim + lowercase + deduplicate`
- `analysisModules` 每個值都必須符合 `^[a-z0-9][a-z0-9_-]*$`
- `analysisModules` 不可包含保留字 `common`
- `subjectProfile` 在 text-only profile request 中可以省略
- 若 `subjectProfile` 有值，則 `subjectId` 必填
- `subjectProfile` 內每個 module payload 都應落在 `analysisModules` 內
- `subjectProfile` 內 `moduleKey` 不可重複
- 同一個 module 內的 `factKey` 不可重複
- `factKey` 不可為空白
- 每個 fact 至少需要一個 value
- `values[]` 內每個 value 都不可為空白
- `theoryVersion` 若提供，不可為空白；是否必填由 app-specific prompt strategy 與對應 module 決定
- 目前 `appId=linkchat` 且 `moduleKey=astrology` 時，`theoryVersion` 必填
- 目前 `appId=linkchat` 且 `moduleKey=mbti` 時，`theoryVersion` 可省略

## Notes
- Gatekeeper 不做附件內容解析
- Gatekeeper 不做附件落地保存
- `appId` 目前作為 external app 的業務授權 key；Cloud Run service-to-service auth 將於部署階段補上
- 尚未實作 IP allowlist / blocklist
- 尚未實作 MIME validation
- Gatekeeper 不負責 LinkChat 的 module entitlement 與缺資料剔除；那是 external app 的本地 gatekeeping
- Gatekeeper 不負責 raw theory value -> Internal private code mapping；那是 builder prompt strategy 與 codebook repository 的責任
- public prompt-testing routes 的安全性預設由部署/環境隔離保護，不由 gatekeeper 在第一版內做 app auth
