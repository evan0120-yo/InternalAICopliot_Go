# Gatekeeper BDD Spec

## Purpose
這份文件定義 gatekeeper module 目前應滿足的行為規格。內容以現有 code、測試與 module spec 為基準。

## Actors
- public user：查詢 builders 或發起 consult
- external app：查詢自己可用的 builders 或發起 consult
- grpcapi transport：將 gRPC request 轉成 gatekeeper 可驗的 command
- gatekeeper handler：解析 HTTP request 並將合法請求交給 usecase
- guard service：驗證 consult request 與解析 client IP
- promptguard usecase：針對 raw user text 做 prompt injection guard 決策

## Scenario Group: List Builders
- Given public user 呼叫 `GET /api/builders`
  When request 進入 gatekeeper handler
  Then 系統應回傳 builder query service 所提供的 active builders 清單

- Given external app 呼叫 `GET /api/external/builders` 並帶入 `X-App-Id`
  When request 進入 gatekeeper handler
  Then 系統應只回傳該 app 被授權且 active 的 builders 清單

- Given external app 呼叫 `GET /api/external/builders` 但缺少 `X-App-Id`
  When request 進入 gatekeeper handler
  Then 應回傳 `APP_ID_MISSING`

## Scenario Group: Generic Consult Request Parsing
```text
HTTP multipart request
      │
      ├─ parse multipart form
      ├─ parse builderId / text / outputFormat
      ├─ read files -> attachments
      ├─ public route
      │   └─ optional appId 只作 strategy hint
      └─ external route
          └─ X-App-Id 走 external app auth
```

- Given multipart form parsing 失敗
  When handler 執行 `ParseMultipartForm`
  Then 應回傳 `INVALID_MULTIPART`

- Given `builderId` 缺失或無法轉為數字
  When handler 解析 form 欄位
  Then 應回傳 `BUILDER_ID_MISSING`

- Given multipart 中有 `files`
  When handler 讀取附件
  Then 每個檔案都應被轉成 `infra.Attachment` 並交給 usecase

- Given 檔案無法開啟或讀取
  When handler 讀取附件 bytes
  Then 應回傳 `FILE_READ_FAILED`

- Given external app 呼叫 `POST /api/external/consult`
  When handler 解析 request
  Then 應使用相同的 multipart parsing 規則讀取 `builderId`、`text`、`outputFormat` 與 `files`

- Given public user 呼叫 `POST /api/consult` 並帶入 optional `appId`
  When handler 解析 request
  Then 應將該 `appId` 視為 prompt strategy hint 傳給 usecase，而不啟動 external app 授權檢查

- Given public user 呼叫 `POST /api/profile-consult` 並帶入 optional `appId`
  When handler 解析 JSON request
  Then 應將該 `appId` 視為 prompt strategy hint 傳給 usecase，而不啟動 external app 授權檢查

- Given `POST /api/profile-consult` 收到非法 JSON
  When handler 解析 request body
  Then 應回傳 `INVALID_JSON`

- Given `POST /api/profile-consult` 帶入不支援的 `mode`
  When handler 解析 JSON request
  Then 應回傳 `INVALID_MODE`

## Scenario Group: Profile Consult Validation
```text
ProfileConsult request
       │
       ├─ builderId 驗證
       ├─ subjectProfile 缺值？
       │   └─ 是 -> 合法 text-only profile
       └─ subjectProfile 有值
           ├─ subjectId 必填
           ├─ analysisPayloads[] 不可重複
           ├─ analysisType 需合法
           └─ theoryVersion 若提供不可空白
```

- Given grpcapi `ProfileConsult` 傳入帶有多個 analysis payload 的 structured `subjectProfile`
  When gatekeeper 驗證 structured profile consult
  Then 應保留 `builderId` 與 `subjectProfile` envelope，並以 `ConsultModeProfile` 轉交 builder usecase

- Given `ProfileConsult` 未帶 `subjectProfile` 但 `text` 有值
  When gatekeeper 驗證 structured profile consult
  Then 不應拒絕該 request，且仍應以 `ConsultModeProfile` 轉交 builder usecase

- Given structured `subjectProfile` 內某個 analysis payload 缺少 `analysisType`
  When gatekeeper 驗證 structured profile consult
  Then 應拒絕該 request

- Given structured `subjectProfile` 內同一個 `analysisType` 出現兩次
  When gatekeeper 驗證 structured profile consult
  Then 應拒絕該 request

- Given structured `subjectProfile` 的某個 payload 帶了空白 `theoryVersion`
  When gatekeeper 驗證 structured profile consult
  Then 應拒絕該 request

- Given `appId=linkchat` 且 `subjectProfile` 內存在 `analysisType=astrology`
  When gatekeeper 與下游 strategy 協作處理 structured profile consult
  Then gatekeeper 不應要求該 payload 必須帶 `theoryVersion`
  And 是否使用 canonical key composable path 應交由下游 strategy 決定

- Given LinkChat 已因缺資料而省略某個 analysis payload
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 不應自行補回或推測該 analysis type

- Given analysis payload 內部是 astrology 專屬 shape 或 mbti 專屬 shape
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 只驗共享 envelope，不負責解析各 analysis type 的內部欄位

- Given `analysisType=astrology` 的某個 slot 採 weighted canonical entries
  And 單一 entry 缺少 `weightPercent`
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 應接受該 payload

- Given `analysisType=astrology` 的某個 slot 採多個 weighted canonical entries
  And 其中某筆缺少 `weightPercent`
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 應拒絕該 request

- Given `analysisType=astrology` 的某個 slot 採多個 weighted canonical entries
  And `weightPercent` 總和不等於 `100`
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 應拒絕該 request

- Given `analysisType=astrology` 的某個 slot 採 weighted canonical entries
  And 某筆 entry 缺少 `key`
  When gatekeeper 驗證 structured profile consult
  Then gatekeeper 應拒絕該 request

## Scenario Group: Client IP Resolution

```text
Client IP 瀑布回退解析流程

  X-Forwarded-For 存在？
       │
       ├── 是 → 取第一個 IP → 回傳 client IP
       │
       └── 否
            │
            ▼
       X-Real-IP 存在？
            │
            ├── 是 → 使用 X-Real-IP → 回傳 client IP
            │
            └── 否
                 │
                 ▼
            回退到 RemoteAddr → 回傳 client IP
```

- Given request 含 `X-Forwarded-For`
  When guard service 解析 client IP
  Then 應取第一個 IP 作為 client IP

- Given request 含 `X-Real-IP`
  When `X-Forwarded-For` 不存在
  Then 應使用 `X-Real-IP`

- Given proxy header 都沒有
  When guard service 解析 client IP
  Then 應回退到 `RemoteAddr`

## Scenario Group: Consult Validation
```text
Consult 驗證主鏈
    │
    ├─ external path ? -> app auth
    ├─ builder validation
    ├─ outputFormat validation
    ├─ file limits validation
    ├─ client IP resolution
    └─ mode 決定
        ├─ generic -> ConsultModeGeneric
        └─ profile -> ConsultModeProfile
```

- Given external API 缺少 `appId`
  When `ValidateExternalApp` 或 `ValidateExternalConsult` 執行
  Then 應回傳 `APP_ID_MISSING`

- Given external app 不存在
  When `ValidateExternalApp` 或 `ValidateExternalConsult` 執行
  Then 應回傳 `APP_NOT_FOUND`

- Given external app 為 inactive
  When `ValidateExternalApp` 或 `ValidateExternalConsult` 執行
  Then 應回傳 `APP_INACTIVE`

- Given client IP 為空
  When `ValidateConsult` 執行
  Then 應回傳 `CLIENT_IP_MISSING`

- Given builderId 為 0
  When `ValidateConsult` 執行
  Then 應回傳 `BUILDER_ID_MISSING`

- Given builder 不存在
  When `ValidateConsult` 執行
  Then 應回傳 `BUILDER_NOT_FOUND`

- Given builder 為 inactive
  When `ValidateConsult` 執行
  Then 應回傳 `BUILDER_INACTIVE`

- Given `outputFormat` 有值但不是 `markdown` 或 `xlsx`
  When `ValidateConsult` 執行
  Then 應回傳 `UNSUPPORTED_OUTPUT_FORMAT`

- Given 附件副檔名不在支援清單
  When `ValidateConsult` 執行
  Then 應回傳 `UNSUPPORTED_FILE_TYPE`

- Given 附件數量、單檔大小或總大小超過 config 限制
  When `ValidateConsult` 執行
  Then 應分別回傳 `FILE_COUNT_EXCEEDED`、`FILE_SIZE_EXCEEDED` 或 `FILE_TOTAL_SIZE_EXCEEDED`

- Given external app 嘗試使用未授權的 builder
  When `ValidateExternalConsult` 執行
  Then 應回傳 `APP_BUILDER_FORBIDDEN`

- Given generic request 合法
  When usecase 執行 consult
  Then 應將 builderId、text、parsed output format、attachments、client IP 與 `ConsultModeGeneric` 轉交給 builder consult usecase

- Given external app generic consult request 合法
  When usecase 執行 external consult
  Then 應先驗 app 權限，再將 builderId、text、parsed output format、attachments、client IP 與 `ConsultModeGeneric` 轉交給 builder consult usecase

- Given external app structured profile consult request 合法
  When usecase 執行 consult
  Then 應將 appId、builderId、optional `subjectProfile`、text、client IP 與 `ConsultModeProfile` 轉交給 builder consult usecase

- Given public generic consult request 未帶 `appId`
  When usecase 執行 consult
  Then 應將空 `appId` 轉交給 builder consult usecase，讓下游使用 default prompt strategy

## Scenario Group: PromptGuard Integration For Profile Consult
```text
ProfileConsult / PublicProfileConsult
        │
        ├─ gatekeeper validation
        ├─ builderCode != linkchat-astrology？ -> 直接 builder consult
        ├─ text 為空？ -> 直接 builder consult
        └─ text 有值
            └─ promptguard usecase
                ├─ allow -> builder consult
                ├─ block -> 正常 business response
                └─ internal failure -> 系統錯誤
```

- Given `ProfileConsult` 或 `PublicProfileConsult` request 已通過 gatekeeper 驗證
  And `text` 有值
  When gatekeeper usecase 執行 profile consult orchestration
  Then 應先呼叫 promptguard usecase
  And 不應直接把 request 送進 builder consult 主流程

- Given profile consult request 的 builder 不是 `linkchat-astrology`
  When gatekeeper usecase 處理 profile consult
  Then 第一版應跳過 promptguard
  And 應直接繼續 builder consult 主流程

- Given promptguard evaluation 回傳 `allow`
  When gatekeeper usecase 繼續執行 profile consult
  Then 應將原本 validated request 轉交給 builder consult usecase

- Given promptguard evaluation 回傳 `block`
  When gatekeeper usecase 處理 profile consult
  Then 不應再呼叫 builder consult usecase
  And 應直接回正常 business response
  And 該 response 應以 `status=false` 表示 blocked，而不是 validation 4xx

- Given promptguard evaluation 過程發生 builder / aiclient / config / parse failure
  When gatekeeper usecase 處理 profile consult
  Then 應將其視為系統錯誤
  And 不應把這類 failure 誤當成 `status=false` 的正常 block

- Given profile consult request 沒有 `text`
  When gatekeeper usecase 處理 profile consult
  Then 應跳過 promptguard
  And 應直接繼續 builder consult 主流程

- Given request 為 generic consult 或 external generic consult
  When gatekeeper usecase 執行 consult
  Then 第一版不應呼叫 promptguard usecase

## Acceptance Notes
- gatekeeper 目前處理 internal API：`GET /api/builders`、`POST /api/consult`
- gatekeeper 目前也處理 public local/dev profile prompt testing API：`POST /api/profile-consult`
- gatekeeper 目前也處理 external API：`GET /api/external/builders`、`POST /api/external/consult`
- gatekeeper 的 usecase / service 也應承接 grpcapi 轉進來的 generic `Consult` 與 `ProfileConsult`
- gatekeeper 不應自行組裝 prompt，也不應直接接觸 repository 細節
- `POST /api/profile-consult` 的 `appId` 只作為 prompt strategy hint，不代表通過 external app auth
- LinkChat analysis-specific payload parsing 應落在 builder 內第二層 factory，而不是 gatekeeper
- promptguard orchestration 應放在 gatekeeper usecase，不應塞進 guard service
- 第一版 promptguard integration 只先套用在 profile astrology 主流程，不擴到 generic consult

## Code-Backed Tests
- `service_test.go`

## Open Questions
- Cloud Run service-to-service auth 尚未接入，現在 external API 僅以 `X-App-Id` 做 app-level 授權
- MIME type 驗證尚未實作，現在以副檔名為主
