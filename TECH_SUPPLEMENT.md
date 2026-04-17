# Internal AI Copilot Go Backend Tech Supplement

這份文件是 CODE_REVIEW.md 的技術補充（原 BLOCK 3）。
面向需要深入了解 call chain、wiring、測試覆蓋的讀者。

> 快速導覽請看 CODE_REVIEW.md 的 BLOCK 1 / BLOCK 2。

---

## A. Startup wiring

主要檔案：
- cmd/api/main.go
- internal/app/app.go

目前 wiring 順序：

```text
app.New
├─ infra.NewStoreWithOptions
├─ rag.NewResolveUseCase
├─ aiclient.NewAnalyzeUseCase
├─ output.NewRenderUseCase
├─ builder.NewConsultUseCase
├─ promptguard.NewEvaluateUseCase
├─ gatekeeper.NewUseCase
├─ gatekeeper.NewHandler
└─ grpcapi.Register
```

## B. Shared consult core

主要檔案：
- internal/gatekeeper/usecase.go
- internal/builder/consult_usecase.go
- internal/builder/task_builder_factory.go
- internal/output/service.go

關鍵 flow：

```text
gatekeeper.UseCase
└─ builder.ConsultUseCase
   ├─ preload or load builder
   ├─ load sources
   ├─ taskBuilder := factory.BuilderFor(command)
   ├─ taskBuilder.PrepareSources(...)
   ├─ resolve rag by sourceID
   ├─ taskBuilder.Build(...)
   ├─ aiclient.Analyze(...)
   └─ output.Render(...)
```

task builder factory 目前三種：
- generic
- profile
- extract

## C. Profile consult + promptguard

主要檔案：
- internal/gatekeeper/usecase.go
- internal/gatekeeper/service.go
- internal/promptguard/service.go
- internal/promptguard/usecase.go

目前 promptguard 進入條件：

```text
shouldRunPromptGuard
├─ userText / intentText 不能全空
└─ builderCode 必須是 linkchat-astrology
```

block 後回的 business response 目前會是：

```text
status       = false
statusAns    = prompts有違法注入內容
response     = 取消回應
responseDetail = guard reason
```

## D. Line task extraction

主要檔案：
- internal/gatekeeper/usecase.go
- internal/grpcapi/service.go
- internal/builder/assemble_service.go
- internal/aiclient/response_contract.go

`buildLineTaskCommand` 目前固定做的事：

```text
buildLineTaskCommand
├─ Mode            = ConsultModeExtract
├─ AIExecutionMode = live
├─ trim appID / messageText
├─ resolveLineTaskExecutionContext
└─ normalizeLineTaskSupportedTaskTypes
```

fallback 規則：

```text
referenceTime 空
└─ 用 backend 現在時間補

timeZone 空
└─ 先取 Location().String()
   └─ 不可用再退到 UTC±HH:MM

supportedTaskTypes 空
└─ ["calendar"]
```

gRPC / HTTP extraction response 最後都會 normalize 成：
- taskType
- operation
- summary
- startAt
- endAt
- location
- missingFields

## E. Admin graph / templates

主要檔案：
- internal/builder/admin_handler.go
- internal/builder/graph_service.go
- internal/builder/template_service.go
- internal/infra/store.go

graph save 語意：

```text
save graph
├─ 忽略 request 裡的 systemBlock source overwrite
├─ 刪掉既有非 systemBlock sources + rags
├─ 寫入新 sources
├─ 寫入新 rags
└─ 回寫 counters / lookup
```

template delete 副作用：

```text
delete template
├─ 刪 templateRags
├─ 刪 template
├─ 清 builder source copiedFromTemplate*
└─ reorder templates
```

## F. Current test baseline

主要檔案：
- internal/app/app_integration_test.go
- internal/gatekeeper/*.go
- internal/builder/*_test.go
- internal/aiclient/*_test.go
- internal/grpcapi/service_test.go
- internal/promptguard/*_test.go

目前 root 級驗證基線：

```text
go test ./...
go vet ./...
```

---

**文件版本**：v1.0
**最後更新**：2026-04-17
**作者**：Claude Sonnet 4.6
