# Internal AI Copilot Go Backend Development Guide

## Scope

這份文件定義 Go backend 的開發流程、文件同步規則與 root/module 文件分工。

這個 repo 的定位是：

```text
Go backend
├─ shared internal AI orchestration core
├─ public / external / gRPC transport
└─ admin graph / template tools
```

## Document Suite

root 文件分工如下：

```text
PLAN.md
└─ 高層範圍、架構方向、長線背景

BDD.md
└─ root 級可觀察行為

SDD.md
└─ root 級模組切法、資料流、依賴方向

TDD.md
└─ root 級測試策略與驗證順序

CODE_REVIEW.md
└─ 根據現有 code 寫出的 current implementation truth
```

package 級細節則維持：

```text
internal/*/*_BDD.md
internal/*/*_spec.md
```

## Primary Development Flow

```text
Step 1: Confirm Behavior
└─ 先看 root BDD 或對應 module BDD

Step 2: Confirm Structure
└─ 再看 root SDD 或對應 module spec

Step 3: Confirm Existing Code
└─ 直接讀 handler / usecase / service / store

Step 4: Map To Tests
└─ 依 TDD.md 決定先補哪層測試

Step 5: Implement Minimum Change
└─ 只改對應 module 與對應層

Step 6: Verify
└─ 至少跑
   ├─ go test ./...
   └─ go vet ./...

Step 7: Sync Docs
└─ 行為、結構或測試基線有變，就回寫文件
```

## Layer Rules

```text
transport
└─ usecase
   └─ service
      └─ infra/store
```

### transport

包含：
- HTTP handler
- grpcapi

責任：
- parse request
- map response
- transport error mapping

不要做：
- store 直接存取
- 業務編排

### usecase

責任：
- 業務編排
- cross-module coordination
- concurrency ownership

### service

責任：
- deterministic business logic
- prompt assembly
- normalization
- policy decision

### infra/store

責任：
- persistence
- config
- seed
- helper

## Current Architecture Rules

### Shared consult core

```text
public HTTP / external HTTP / gRPC
└─ gatekeeper
   └─ builder consult
      ├─ rag
      ├─ aiclient
      └─ output
```

規則：
- transport 可以不同，核心 orchestration 要收斂
- generic / profile / extract 走同一個 builder consult 主幹

### PromptGuard

規則：
- promptguard 是獨立 module
- gatekeeper 只決定是否先跑 guard
- guard 的實際評估流程由 promptguard 自己負責

### Line task extraction

規則：
- local/dev HTTP route 與 gRPC route 要對齊同一套核心 contract
- line task extraction 不要塞回 ProfileConsult
- extract flow 預設強制 live mode

## Verification Baseline

目前根級驗證基線：

```text
go test ./...
go vet ./...
```

若只改單一模組，可先跑 targeted package，再回到全量。

## Sync Rules

### 行為改動

至少同步：
- BDD.md
- CODE_REVIEW.md
- 對應 module `*_BDD.md`

### 結構改動

至少同步：
- SDD.md
- CODE_REVIEW.md
- 對應 module `*_spec.md`

### 測試基線改動

至少同步：
- TDD.md
- CODE_REVIEW.md

## Commands

```text
go test ./...
go vet ./...
go run .\cmd\api
```
