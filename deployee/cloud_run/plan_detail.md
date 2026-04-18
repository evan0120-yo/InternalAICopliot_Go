# Internal AI Copilot — Cloud Run Deployment Detail

---

# BLOCK 1: 部署目標與前提

這份文件是 `plan.md` 的細節補充，針對每個部署步驟做深度說明。

**部署目標：**
- 把 InternalAICopliot Go backend 作為 Cloud Run 服務跑起來
- 同時接受 HTTP（REST）和 gRPC 呼叫
- 生產環境連接真正的 Firestore（not emulator）
- 不對外公開，只允許擁有 `roles/run.invoker` 的 SA 呼叫

**前提條件：**
```text
prerequisites
├─ gcloud SDK 已安裝且 auth login 完成
├─ docker 已安裝
├─ Artifact Registry repo 已建立
│  └─ gcloud artifacts repositories create docker-repo
│     --repository-format=docker --location=asia-east1 --project=dailo-467502
├─ Service Account 已建立
│  └─ internal-ai-copilot@dailo-467502.iam.gserviceaccount.com
├─ IAM 綁定完成
│  └─ roles/datastore.user 給上面的 SA
└─ docker auth 設定
   └─ gcloud auth configure-docker asia-east1-docker.pkg.dev
```

---

# BLOCK 2: 完整部署流程圖

```text
本機開發環境
      │
      │  git pull / 程式碼確認
      ▼
┌─────────────────────────────────────────────────┐
│  Step 1: Build Docker Image                     │
│                                                 │
│  docker build \                                 │
│    -f deployee/cloud_run/Dockerfile \           │
│    -t [REGISTRY]/internal-ai-copilot:latest .   │
│                                                 │
│  Build stages:                                  │
│  golang:1.25-alpine (builder)                   │
│  └─ go mod download                             │
│  └─ CGO_ENABLED=0 go build -o internal-ai-server│
│  alpine:3.21 (runtime)                          │
│  └─ COPY binary                                 │
│  └─ apk add ca-certificates tzdata             │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  Step 2: Push to Artifact Registry              │
│                                                 │
│  docker push [REGISTRY]/internal-ai-copilot:latest│
│                                                 │
│  Registry:                                      │
│  asia-east1-docker.pkg.dev/                     │
│    dailo-467502/docker-repo/                    │
│      internal-ai-copilot:latest                 │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  Step 3: Deploy to Cloud Run                    │
│                                                 │
│  gcloud run deploy internal-ai-copilot \        │
│    --image [image] \                            │
│    --region asia-east1 \                        │
│    --service-account [SA] \                     │
│    --no-allow-unauthenticated \                 │
│    --port 8080 \                                │
│    --set-env-vars "..."                         │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
        Cloud Run 服務上線
        取得 Service URL
        格式: https://internal-ai-copilot-XXXXX-de.a.run.app
                  │
                  ▼
        提供給 LineBot 部署時使用
        LINEBOT_INTERNAL_GRPC_ADDR = [URL]:443
```

## h2c 路由邏輯（container 內部）

```text
Cloud Run 接收請求 (HTTPS，TLS 由平台終止)
      │
      ▼ (plain HTTP/2 進 container)
h2c.NewHandler(combined, &http2.Server{})
      │
      ├─ r.ProtoMajor == 2
      │  AND Content-Type starts with "application/grpc"
      │  │
      │  └─▶ grpc.Server.ServeHTTP(w, r)
      │       └─ IntegrationService.LineTaskConsult(...)
      │
      └─ else
         └─▶ httpMux.ServeHTTP(w, r)
              ├─ GET  /health → 200 OK
              ├─ POST /api/consult
              └─ POST /api/...
```

## Firestore 連線邏輯

```text
container 啟動
      │
      ▼
infra.NewFirestoreClient(projectID)
      │
      ├─ FIRESTORE_EMULATOR_HOST != ""
      │  └─▶ 連 emulator（local dev 用）
      │
      └─ FIRESTORE_EMULATOR_HOST == "" (生產預設)
         └─▶ Application Default Credentials
              └─ Cloud Run SA: internal-ai-copilot@dailo-467502.iam.gserviceaccount.com
                   └─ roles/datastore.user → 讀寫 Firestore
```

---

# BLOCK 3: 技術補充

## 環境變數完整清單

| 變數名 | 必填 | 生產值 | 本機值 |
|--------|------|--------|--------|
| `PORT` | 自動注入 | `8080` | — |
| `GEMINI_API_KEY` | ✅ | Secret Manager 或直接設定 | 本機 key |
| `OPENAI_API_KEY` | 可空 | `""` | `""` |
| `INTERNAL_AI_COPILOT_AI_PROFILE` | ✅ | `1` | `1` |
| `INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID` | ✅ | `dailo-467502` | `dailo-467502` |
| `FIRESTORE_EMULATOR_HOST` | ❌ 生產不設 | 不設定 | `localhost:8090` |

## Service Account 設定

```text
SA name: internal-ai-copilot@dailo-467502.iam.gserviceaccount.com

IAM roles:
├─ roles/datastore.user
│  └─ 允許 Firestore CRUD
└─ (不需要 Storage, Pub/Sub 等其他 role)
```

## Artifact Registry

```text
Registry: asia-east1-docker.pkg.dev
Project:  dailo-467502
Repo:     docker-repo
Image:    internal-ai-copilot
Tag:      latest (或 git commit sha)

Full path:
asia-east1-docker.pkg.dev/dailo-467502/docker-repo/internal-ai-copilot:latest
```

## Docker Image 結構

```text
Build stage (golang:1.25-alpine)
├─ WORKDIR /app
├─ COPY go.mod go.sum → go mod download (layer cache)
├─ COPY . .
└─ go build → /app/internal-ai-server

Runtime stage (alpine:3.21)
├─ apk add ca-certificates  ← TLS 握手需要
├─ apk add tzdata            ← 時區計算需要
├─ COPY binary from builder
├─ EXPOSE 8080
└─ CMD ["./internal-ai-server"]
```

> `CGO_ENABLED=0`：純靜態 binary，不依賴 libc，確保 alpine 可以執行。
> `GOOS=linux`：在 Windows/Mac build machine 也能產出 Linux binary。

## Concurrency 設定建議

Cloud Run 預設每個 instance 可同時處理 80 requests。
AI 呼叫（Gemini）單次可能耗時 5-30 秒，建議：

```text
--concurrency 10        # 每個 instance 同時最多 10 個請求
--min-instances 0       # 允許縮到 0（省費用）
--max-instances 5       # 避免 Gemini API quota 超限
```

## 已知限制

```text
限制
├─ 沒有 warm-up：cold start 約 2-4 秒（alpine image 較小，OK）
├─ gRPC streaming 未使用，目前只有 unary call
└─ Secret Manager 尚未整合（API key 直接帶在 env vars）
```

## 本機開發指令

```cmd
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go"
set "GEMINI_API_KEY=xxx"
set "OPENAI_API_KEY="
set "INTERNAL_AI_COPILOT_AI_PROFILE=1"
set "FIRESTORE_EMULATOR_HOST=localhost:8090"
go run .\cmd\api
```
