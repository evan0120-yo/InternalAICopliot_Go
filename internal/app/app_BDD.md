# App BDD Spec

## Purpose
這份文件定義 app wiring layer 目前應滿足的整合層行為規格。`app` 不是業務模組，而是所有 module 的組裝與 HTTP / gRPC surface 入口。

## Actors
- runtime bootstrap：呼叫 `app.New` 建立整個應用
- frontend / API caller：透過最終 HTTP handler 存取 internal、external 與 admin routes
- external gRPC caller：例如 LinkChat，透過 `RegisterGRPC` 掛出的 integration service 呼叫 Internal

## Scenario Group: Application Wiring
- Given config 合法
  When `app.New` 執行
  Then 系統應建立 store、rag、aiclient、output、builder、gatekeeper 依賴，並將 public/admin routes 註冊到同一個 mux

- Given app 已建立完成
  When `RegisterGRPC` 執行
  Then 應將 integration gRPC service 綁到同一個 gatekeeper usecase，而不是建立另一套 consult 流程

- Given store 初始化失敗
  When `app.New` 執行
  Then 應直接回傳 error，不建立 app

## Scenario Group: Middleware
- Given任何 request 經過 app handler
  When `withCORS` 執行
  Then response header 應依 configured allowed origins 回傳允許的 headers 與 methods

- Given request method 為 `OPTIONS`
  When `withCORS` 執行
  Then 應直接回傳 `204 No Content`

- Given下游 handler panic
  When `withPanicRecovery` 捕捉 panic
  Then 應記錄 stack trace，並以 `INTERNAL_SERVER_ERROR` JSON envelope 回應

## Scenario Group: End-to-End Public Flow
- Given app 已成功啟動
  When caller 執行 `GET /api/builders`
  Then 應得到成功回應

- Given app 已成功啟動且 consult request 合法
  When caller 執行 `POST /api/consult`
  Then consult flow 應成功跑完，且若 builder 需要輸出檔案，response 中應帶有對應檔案 payload

- Given app 已成功啟動且 local/dev 測試用的 line task request 合法
  When caller 執行 `POST /api/line-task-consult`
  Then request 應進入同一套 gatekeeper / builder / aiclient extraction 流程

- Given runtime builder list 包含 `line-memo-crud`
  When Internal frontend 以該 builder 進入 line task 測試畫面
  Then submit 應命中 `POST /api/line-task-consult`
  And 不應退回 generic `POST /api/consult`

## Scenario Group: End-to-End External HTTP Flow
- Given app 已成功啟動且 external app 帶入合法 `X-App-Id`
  When caller 執行 `GET /api/external/builders`
  Then 應得到該 app 被授權的 active builders 清單

- Given app 已成功啟動且 external consult request 合法
  When caller 執行 `POST /api/external/consult`
  Then consult flow 應成功跑完，且 app 只能使用自己被授權的 builder

## Scenario Group: End-to-End External gRPC Flow
- Given app 已成功啟動且 LinkChat 透過 gRPC 帶入固定 `builderId`、`analysisModules`、`subjectProfile` 與 optional `text`
  When caller 執行 gRPC `ProfileConsult`
  Then 請求應進入同一套 gatekeeper / builder / aiclient / output consult 流程

## Scenario Group: End-to-End Admin Flow
- Given app 已成功啟動
  When caller 執行 `POST /api/admin/templates`
  Then 建立成功時 HTTP status 應為 `201 Created`

## Code-Backed Tests
- `app_integration_test.go`

## Open Questions
- app 層目前沒有獨立 spec 文件，只靠這份 BDD 描述整合行為；若未來 app wiring 變複雜，可能需要補 `app_spec.md`
