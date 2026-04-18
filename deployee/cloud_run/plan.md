# Internal AI Copilot — Cloud Run Deployment Plan

---

# BLOCK 1: 這個服務是什麼

這個 Go backend 看起來像一個「**可設定的 AI 諮詢引擎**」。
部署到 Cloud Run 之後，它同時扮演兩個角色：

- **HTTP API server**：對內前台查詢、外部系統 REST 呼叫
- **gRPC server**：接收 LineBot 等後端服務的結構化諮詢請求

關鍵設計選擇：
- 單一 PORT 同時服務 HTTP + gRPC（h2c 多工），符合 Cloud Run 單埠限制
- Firestore 作為持久化，生產環境用 Application Default Credentials（不帶 emulator host）
- Stateless container，每個 instance 可獨立擴縮
- 不允許未認證請求（`--no-allow-unauthenticated`），service-to-service 走 Google OIDC

---

# BLOCK 2: Cloud Run 架構圖

```text
┌─────────────────────────────────────────────────────────┐
│  Cloud Run: internal-ai-copilot                         │
│                                                         │
│  PORT (8080, from Cloud Run env)                        │
│      │                                                  │
│      ▼                                                  │
│  h2c.NewHandler                                         │
│  ├─ Content-Type: application/grpc → grpcServer         │
│  └─ else                          → httpMux             │
│                                                         │
│  grpcServer                                             │
│  └─ IntegrationService                                  │
│     └─ LineTaskConsult                                  │
│                                                         │
│  httpMux                                                │
│  ├─ GET  /health                                        │
│  ├─ POST /api/consult                                   │
│  └─ POST /api/...                                       │
└───────────────┬─────────────────────────────────────────┘
                │
                ▼
┌──────────────────────────────┐
│  Firestore (asia-east1)      │
│  Project: dailo-467502       │
│  Mode: Native                │
└──────────────────────────────┘

Caller side
┌──────────────────────────────┐
│  LineBot (Cloud Run)         │──── gRPC (TLS) ──────────▶  internal-ai-copilot
│  LINEBOT_INTERNAL_GRPC_INSECURE=false                    (上方服務)
└──────────────────────────────┘
```

**IAM 邊界：**
```text
internal-ai-copilot SA
├─ roles/datastore.user      → 讀寫 Firestore
└─ (被呼叫端)
   └─ LineBot SA 需要 roles/run.invoker 才能呼叫
```

**Artifact Registry 路徑：**
```text
asia-east1-docker.pkg.dev/dailo-467502/docker-repo/internal-ai-copilot:latest
```

---

# BLOCK 3: 補充細節

## 必要環境變數

| 變數名 | 說明 | 範例 |
|--------|------|------|
| `PORT` | Cloud Run 自動注入 | `8080` |
| `GEMINI_API_KEY` | Gemini AI key | `AIza...` |
| `OPENAI_API_KEY` | OpenAI key（可空） | `""` |
| `INTERNAL_AI_COPILOT_AI_PROFILE` | AI profile 設定 | `1` |
| `INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID` | GCP project | `dailo-467502` |

> `FIRESTORE_EMULATOR_HOST` 在生產不設定，預設 `""`，自動走 ADC 認證。

## Build → Push → Deploy 步驟

```text
Step 1: docker build
└─ context: InternalAICopliot/Backend/Go/
└─ Dockerfile: deployee/cloud_run/Dockerfile
└─ tag: asia-east1-docker.pkg.dev/dailo-467502/docker-repo/internal-ai-copilot:latest

Step 2: docker push
└─ 需要先 gcloud auth configure-docker asia-east1-docker.pkg.dev

Step 3: gcloud run deploy
└─ --service-account internal-ai-copilot@dailo-467502.iam.gserviceaccount.com
└─ --no-allow-unauthenticated
└─ --port 8080
```

## 注意事項

> `golang.org/x/net` 已從 indirect 升為 direct（h2c 直接使用）。
> alpine image 需要 `ca-certificates` 才能做 TLS 握手（已加入 Dockerfile）。
> Cloud Run 外部 TLS 由平台終止，container 內走明文 HTTP/2（h2c）。
