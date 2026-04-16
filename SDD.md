# Internal AI Copilot Go Backend SDD

## Scope

這份文件只描述 Go backend root 級結構、模組責任、資料流與依賴方向。

更細的 package 規則，仍以各模組自己的 `*_spec.md` 與 `*_BDD.md` 為準。

## Runtime Shape

```text
cmd/api
└─ internal
   ├─ app
   ├─ grpcapi
   ├─ gatekeeper
   ├─ promptguard
   ├─ builder
   ├─ rag
   ├─ aiclient
   ├─ output
   └─ infra
```

## Module Responsibilities

### app

```text
app
├─ config bootstrap
├─ store bootstrap
├─ module wiring
├─ HTTP register
└─ gRPC register
```

責任：
- 在單一入口把依賴接好
- 建立 HTTP 與 gRPC transport

### grpcapi

```text
grpcapi
├─ protobuf adapter
├─ request mapping
└─ gRPC error mapping
```

責任：
- 把 protobuf request 轉成 gatekeeper command
- 把 business error 映射成 gRPC status

### gatekeeper

```text
gatekeeper
├─ transport-facing validation
├─ external app validation
├─ promptguard dispatch
└─ builder usecase entry
```

責任：
- 擋 request boundary 問題
- 區分 public / external / local-dev 路徑
- 決定何時先跑 promptguard

### promptguard

```text
promptguard
├─ text normalize
├─ rule matching
├─ score / decision routing
└─ gray-zone LLM guard
```

責任：
- 專責 prompt injection / override 類 guard

### builder

```text
builder
├─ consult orchestration
├─ task builder factory
├─ prompt assembly
├─ graph service
└─ template service
```

責任：
- 核心業務 orchestration
- source/rag/template 參與規則
- task route 決策

### rag

```text
rag
└─ source-level supplement resolve
```

責任：
- 根據 sourceID 取回已配置的 rag supplements

### aiclient

```text
aiclient
├─ execution mode
├─ route executor
├─ provider call
└─ structured response parse
```

責任：
- preview / mock / live
- OpenAI / Gemma / staged route
- response contract parse

### output

```text
output
├─ output policy
└─ markdown / xlsx render
```

責任：
- 最終決定是否產檔
- 把 business response 轉成 file payload

### infra

```text
infra
├─ Firestore store
├─ config
├─ seed
├─ error
└─ HTTP helpers
```

責任：
- persistence 與基礎設施

## Main Data Flows

### A. Startup Wiring

```text
cmd/api/main.go
└─ app.New
   ├─ infra.NewStoreWithOptions
   ├─ rag.NewResolveUseCase
   ├─ aiclient.NewAnalyzeUseCase
   ├─ output.NewRenderUseCase
   ├─ builder.NewConsultUseCase
   ├─ promptguard.NewEvaluateUseCase
   ├─ gatekeeper.NewUseCase
   └─ register HTTP + gRPC
```

### B. Shared Consult Core

```text
HTTP or gRPC
└─ gatekeeper.UseCase
   └─ builder.ConsultUseCase
      ├─ load builder + sources
      ├─ task builder factory
      ├─ resolve rag
      ├─ assemble prompt
      ├─ aiclient.Analyze
      └─ output.Render
```

關鍵點：
- public HTTP、external HTTP、gRPC generic consult 最後共用同一條 builder consult 主幹

### C. Profile Consult

```text
profile request
└─ gatekeeper.ValidateProfileConsult
   ├─ normalize subjectProfile
   ├─ shouldRunPromptGuard?
   └─ builder consult with ConsultModeProfile
```

關鍵點：
- promptguard 目前只掛在 `linkchat-astrology` profile path

### D. Line Task Extraction

```text
local/dev HTTP or gRPC LineTaskConsult
└─ gatekeeper line-task validation
   └─ buildLineTaskCommand
      ├─ force AIExecutionModeLive
      ├─ resolve referenceTime/timeZone
      ├─ normalize supportedTaskTypes
      └─ builder consult with ConsultModeExtract
```

關鍵點：
- local/dev HTTP route 不做 external app auth
- external gRPC route 會驗 appId 與 allowed builders

### E. Admin Graph / Templates

```text
admin handler
├─ graph usecase
│  ├─ load graph
│  └─ save graph
└─ template usecase
   ├─ list
   ├─ create
   ├─ update
   └─ delete
```

## Dependency Direction

```text
transport
└─ usecase
   └─ service
      └─ infra/store

cross-module orchestration
└─ 只允許在 usecase 層做
```

規則：
- handler / grpcapi 不直接碰 store
- service 不回頭依賴 transport
- repository 能力集中在 infra/store

## Persistence Baseline

```text
apps/{appId}
appPromptConfigs/{appId}
builders/{builderId}
builders/{builderId}/sources/{sourceId}
builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}
templates/{templateId}
templates/{templateId}/templateRags/{templateRagId}
_meta/counters
_sourceLookup/{sourceId}
```

## Current Constraints

```text
current constraints
├─ admin auth 尚未落地
├─ app prompt config cache 沒有 active invalidation
├─ template delete 不是 transaction
├─ source graph 沒有 cycle guard
└─ promptguard 目前只覆蓋特定 profile path
```
