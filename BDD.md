# Internal AI Copilot Go Backend BDD

## Scope

這份文件只描述 Go backend root 級目前已落地的可觀察行為。

更細的 package 行為，仍以各模組自己的 `*_BDD.md` 為準。

## Actors

- public user
- external app
- internal frontend tester
- admin user

## Scenario Group: Public Builder Discovery

```text
GET /api/builders
└─ active builders only
```

- Given public caller 查詢 builders
  When request 成功
  Then response 只應包含 active builders

- Given store 內同時有 active 與 inactive builders
  When public list builders
  Then inactive builders 不應出現在 response

## Scenario Group: External Builder Discovery

```text
gRPC / external builder list
└─ app validation
   └─ allowedBuilderIds filter
```

- Given external app 查詢可用 builders
  When appId 合法且 active
  Then response 只應包含該 app 被授權的 active builders

- Given appId 不存在或 inactive
  When external caller 查詢 builders
  Then request 應被拒絕

## Scenario Group: Generic Consult

```text
consult request
└─ validate
   └─ builder/source/rag load
      └─ assemble prompt
         └─ aiclient
            └─ output render
```

- Given public 或 external caller 發起 generic consult
  When builder 存在且 active
  Then 系統應進入同一條 builder consult orchestration

- Given builder 不存在或 inactive
  When generic consult request 進入
  Then request 應被拒絕

- Given outputFormat 非法
  When generic consult request 進入
  Then request 應被拒絕

- Given attachments 超過數量、大小或副檔名限制
  When generic consult request 進入
  Then request 應被拒絕

- Given builder 需要輸出檔案且 business response 成功
  When output render 執行
  Then response 可包含 file payload

- Given builder 不需要輸出檔案或 response 不是成功結果
  When output render 執行
  Then response 不應包含 file payload

## Scenario Group: Profile Consult

```text
profile consult
└─ subjectProfile normalize
   ├─ promptguard? 
   └─ builder profile path
```

- Given public caller 呼叫 /api/profile-consult
  When request 合法
  Then 系統應走 profile consult flow，不應退回 generic consult

- Given external app 呼叫 gRPC ProfileConsult
  When appId 合法且 request 合法
  Then 系統應走 external profile flow

- Given userText、intentText 與 normalized subjectProfile 同時為空
  When profile consult request 進入
  Then request 應被拒絕

- Given builderCode = linkchat-astrology
  And userText 或 intentText 任一非空
  When profile consult request 進入
  Then 系統應先執行 promptguard

- Given promptguard 決定 block
  When profile consult request 進入
  Then 系統應回 business response
  And 不應繼續進主分析流程

## Scenario Group: Line Task Extraction

```text
LineTaskConsult
├─ local/dev HTTP /api/line-task-consult
└─ external gRPC LineTaskConsult
```

- Given local/dev tester 呼叫 POST /api/line-task-consult
  When request 合法
  Then 系統應走 extract consult flow
  And 不應要求 external app auth

- Given external gRPC caller 呼叫 LineTaskConsult
  When appId 合法且 builder 被授權
  Then 系統應走 extract consult flow

- Given referenceTime 與 timeZone 未提供
  When line task request 進入
  Then backend 應自動補 concrete referenceTime 與 concrete timeZone

- Given supportedTaskTypes 未提供
  When line task request 進入
  Then backend 應預設為 ["calendar"]

- Given line task extraction flow 已送到 AI
  When response 返回
  Then typed result 應包含 taskType
  And typed result 應包含 operation
  And typed result 應包含 eventId
  And typed result 應包含 summary
  And typed result 應包含 startAt
  And typed result 應包含 endAt
  And typed result 應包含 queryStartAt
  And typed result 應包含 queryEndAt
  And typed result 應包含 location
  And typed result 應包含 missingFields

## Scenario Group: Admin Graph

```text
GET graph
PUT graph
```

- Given admin user 載入 graph
  When builder 存在
  Then response 應包含 builder、sources、source rags

- Given admin user 儲存 graph
  When request 合法
  Then非 systemBlock sources 應以整批替換方式重寫

- Given request source 為 systemBlock
  When save graph
  Then request 中該 source 不應覆寫既有 systemBlock source

## Scenario Group: Template Library

```text
GET /api/admin/templates
POST /api/admin/templates
PUT /api/admin/templates/{templateId}
DELETE /api/admin/templates/{templateId}
```

- Given admin user 查詢 templates
  When request 成功
  Then response 應回傳目前 template library

- Given admin user 建立或更新 template
  When request 合法
  Then template 與 template rags 應被保存

- Given admin user 刪除 template
  When delete 成功
  Then template 應被移除
  And copied-from metadata 清理流程應被執行

## Scenario Group: Current Verification Baseline

- Given backend repo 現況
  When 檢視 root 與 internal package tests
  Then 專案應已有 `_test.go` 測試檔
  And 日常驗證基線應為 `go test ./...` 與 `go vet ./...`
