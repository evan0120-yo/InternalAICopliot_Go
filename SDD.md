# Internal AI Copilot Go Backend SDD

## Target Topology

```text
┌────────────────────────────────────────────┐
│  Transport Layer                           │
│  HTTP (public / external / local-dev)      │
│  gRPC (GenericConsult / ProfileConsult /   │
│        LineTaskConsult / ListBuilders)     │
│  Admin HTTP (graph / template CRUD)        │
└───────────────────┬────────────────────────┘
                    │
                    ▼
┌───────────────────────────────────────────┐
│  gatekeeper                               │
│  transport validation · app auth ·        │
│  promptguard dispatch · entry routing     │
└──────────────┬────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│  builder                                 │
│  consult orchestration · task factory ·  │
│  prompt assembly · graph/template admin  │
└────────┬─────────────────┬───────────────┘
         │                 │
         ▼                 ▼
┌─────────────┐    ┌────────────────┐
│  aiclient   │    │  output        │
│  route exec │    │  file policy · │
│  AI call ·  │    │  xlsx/md render│
│  parse resp │    └────────────────┘
└─────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│  infra                                   │
│  Firestore store · config · error · seed │
└──────────────────────────────────────────┘
```

Target: builder-driven AI consultation engine. No session memory, no multi-turn chat.

## Boundary Walls + Runtime Skeleton

### Boundary Walls

```text
must not cross
├─ handler / grpcapi must not touch infra/store directly
├─ service must not import transport packages
└─ promptguard must not call builder — one-way dispatch only

allowed direction
├─ transport → gatekeeper → builder → aiclient → infra
├─ builder → rag → infra
├─ builder → output (render only)
├─ gatekeeper → promptguard (conditional, one-way)
└─ app → all (wiring only, no business logic)
```

### Runtime Skeleton

```text
HTTP / gRPC request
└─ gatekeeper.UseCase
   ├─ validate transport contract (format / attachments / appId)
   ├─ [profile path] promptguard.Evaluate → block → return business response
   └─ builder.ConsultUseCase
      ├─ load builder + sources
      ├─ task builder factory (generic / profile / extract)
      ├─ rag.Resolve
      ├─ assemble prompt
      ├─ aiclient.Analyze
      └─ output.Render → text response [+ file payload]

Admin HTTP request
└─ builder graph/template usecase
   ├─ graph: load (builder + sources + rags) / save (non-system wholesale replace)
   └─ template: list / create / update / delete
```

### Package Map

```text
cmd/api          → entrypoint
internal/app     → startup wiring, HTTP + gRPC register
internal/grpcapi → protobuf adapter, gRPC error mapping
internal/gatekeeper → validation, routing, promptguard dispatch
internal/promptguard → text normalize, rule match, LLM guard
internal/builder → consult orchestration, task factory, prompt assembly, graph/template admin
internal/rag     → source-level RAG supplement resolve
internal/aiclient → execution mode, provider call, response parse
internal/output  → file policy, markdown/xlsx render
internal/infra   → Firestore, config, error, seed helpers
```
