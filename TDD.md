# Internal AI Copilot Go Backend TDD

## Scope

這份文件定義 Go backend root 級測試策略、測試順序與目前測試基線。

## Current Baseline

目前 repo 已有多個 package-level `_test.go`：

```text
已有測試的主要 package
├─ internal/app
├─ internal/gatekeeper
├─ internal/builder
├─ internal/aiclient
├─ internal/grpcapi
├─ internal/output
├─ internal/infra
├─ internal/rag
└─ internal/promptguard
```

目前日常驗證基線：

```text
go test ./...
go vet ./...
```

## Testing Order

### 1. UseCase First

```text
新行為進來
└─ 先找對應 usecase
   ├─ gatekeeper.UseCase
   ├─ builder.ConsultUseCase
   ├─ builder.GraphUseCase
   ├─ builder.TemplateUseCase
   └─ promptguard.EvaluateUseCase
```

這層優先鎖：
- flow 是否走對
- dependency 是否被正確呼叫
- 錯誤是否在正確位置被擋下

### 2. Service Second

```text
service tests
├─ assemble
├─ validation
├─ promptguard scoring
├─ graph normalize
└─ output policy
```

這層優先鎖：
- deterministic business logic
- 邊界條件
- normalize / ordering

### 3. Transport Third

```text
transport tests
├─ gatekeeper HTTP handler
└─ grpcapi service
```

這層優先鎖：
- request parse
- response mapping
- error mapping

### 4. Infra / Store Fourth

```text
infra tests
├─ config
├─ errors
├─ http helpers
├─ store
└─ types
```

這層優先鎖：
- persistence shape
- transaction behavior
- helper correctness

## Change-Type Matrix

### A. 改 Generic Consult / Profile Consult

最小測試集合：

```text
consult change
├─ internal/gatekeeper
├─ internal/builder
├─ internal/aiclient
└─ 視需要 internal/output
```

### B. 改 Line Task Extraction

最小測試集合：

```text
line task change
├─ internal/gatekeeper
├─ internal/builder
├─ internal/aiclient
├─ internal/grpcapi
└─ internal/app
```

必查：
- supportedTaskTypes default
- referenceTime / timeZone fallback
- typed extraction response parse
- typed response new fields (`eventId / queryStartAt / queryEndAt`)
- local/dev HTTP route 與 gRPC route 是否同步

### C. 改 PromptGuard

最小測試集合：

```text
promptguard change
├─ internal/promptguard
├─ internal/gatekeeper
└─ 視需要 internal/aiclient
```

### D. 改 Graph / Templates

最小測試集合：

```text
admin change
├─ internal/builder
├─ internal/infra
└─ 視需要 internal/app
```

## Red-Green-Refactor Rule

```text
1. 先對齊 BDD
2. 先補或修改 failing test
3. 再補最小實作讓它轉綠
4. 最後重構但不改 acceptance
```

## Package Hints

### internal/builder

優先放：
- consult orchestration
- task builder factory
- assemble service
- graph/template rules

### internal/gatekeeper

優先放：
- validation
- public/external 分流
- line task command build
- promptguard dispatch

### internal/aiclient

優先放：
- execution mode
- response contract parse
- provider-specific parsing

### internal/grpcapi

優先放：
- protobuf mapping
- status mapping
- extraction response normalization

## Verification Commands

```text
go test ./...
go vet ./...
```

若只改單一模組，可先跑目標 package，再回到全量：

```text
go test ./internal/gatekeeper
go test ./internal/builder
go test ./internal/aiclient
```
