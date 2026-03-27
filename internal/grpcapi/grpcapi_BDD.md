# GRPC API BDD Spec

## Purpose
這份文件定義 grpcapi module 目前與 profile-analysis contract 應滿足的行為規格。

## Actors
- external app：例如 LinkChat，透過 gRPC 呼叫 Internal
- grpcapi service：gRPC transport adapter
- gatekeeper usecase：承接 transport 已轉換的 consult command

## Scenario Group: List Builders
- Given external app 呼叫 `ListBuilders`
  When grpcapi service 執行
  Then 應依 `appId` 是否存在決定走 public builders 或 external builders 流程

- Given LinkChat profile-analysis hot path
  When 平常發起 consult
  Then 不應要求先呼叫 `ListBuilders`

## Scenario Group: Generic Consult Adaptation
```text
gRPC Consult
    │
    ├─ map request fields
    ├─ map attachments bytes
    ├─ keep appId as-is
    └─ set ConsultModeGeneric
```

- Given external app 傳入 generic `Consult` request
  When grpcapi `Consult` 執行
  Then 應將其轉成交由 gatekeeper / builder 使用的 generic consult command

- Given generic `Consult` request 未帶 `appId`
  When grpcapi `Consult` 執行
  Then 下游應收到空 `appId`，以便使用 default prompt strategy

- Given grpcapi `Consult` 執行
  When command 被建立
  Then command `Mode` 應為 `ConsultModeGeneric`

## Scenario Group: Profile Consult Adaptation
```text
gRPC ProfileConsult
      │
      ├─ keep appId / builderId
      ├─ map subjectProfile envelope
      ├─ map analysisPayloads[]
      │   ├─ analysisType
      │   ├─ optional theoryVersion
      │   └─ payload
      ├─ fallback clientIp
      └─ set ConsultModeProfile
```

- Given external app 傳入固定 `builderId`、optional `subjectProfile` 與 optional `text`
  When grpcapi `ProfileConsult` 執行
  Then 應將其轉成交由 gatekeeper / builder 使用的 structured profile consult command

- Given `ProfileConsult` request 帶 `appId=linkchat`
  When grpcapi 建立 command
  Then 應將該 `appId` 原樣傳給 gatekeeper / builder，不在 grpcapi 內改寫成其他策略 key

- Given `ProfileConsult` request 的某個 analysis payload 帶 `theoryVersion`
  When grpcapi 建立 command
  Then 應保留該欄位並原樣往下傳，不在 grpcapi 內賦予 lookup 語意

- Given `ProfileConsult` request 的某個 analysis payload 內帶 weighted canonical entries
  When grpcapi 建立 command
  Then 應保留 `{key, weightPercent}` 物件陣列形狀
  And 不應在 grpcapi 內先 flatten 成純字串陣列

- Given `ProfileConsult` request 未帶 `subjectProfile` 且 `text` 有值
  When grpcapi 建立 command
  Then command `Mode` 仍應為 `ConsultModeProfile`

- Given grpcapi `ProfileConsult` 執行
  When command 被建立
  Then 不得靠 `subjectProfile` 是否為空推斷 mode

- Given gRPC request 已明確帶 `clientIp`
  When grpcapi 執行
  Then 應使用 request 內的 `clientIp`

- Given gRPC request 未帶 `clientIp`
  When grpcapi 執行
  Then 應回退到 peer address；若仍不可得，則使用 transport fallback 值

## Scenario Group: Error Mapping
```text
gatekeeper / builder business error
        │
        ▼
grpc status code mapping
        │
        └─ ErrorInfo.reason = business error code
```

- Given gatekeeper 或 builder 回傳 business error
  When grpcapi 回應 gRPC caller
  Then 應映射成對應的 gRPC status code，並在 `ErrorInfo.reason` 放入 business error code

## Scenario Group: Generic File Payload
- Given generic builder 仍可能回傳 file payload
  When grpcapi `Consult` 執行成功
  Then 應將 file bytes 直接放在 gRPC `ConsultResponse.file`

## Scenario Group: Profile Text Response
- Given `ProfileConsult` 執行成功
  When grpcapi 回應 gRPC caller
  Then 應回傳純文字 response，不要求 file payload

## Acceptance Notes
- grpcapi 應承接 LinkChat profile-analysis 的 structured transport contract。
- grpcapi 不應實作 LinkChat 的本地 gatekeeping 規則。
- grpcapi 不應實作 raw value -> Internal private code mapping。
