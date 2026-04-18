# Internal AI Copilot — Cloud Run Deployment Log

記錄每次部署的過程、結果、踩坑與修正。

---

## 部署狀態總覽

```text
部署流程
├─ [x] Step 0: 前置準備
│  ├─ [x] gcloud auth login
│  ├─ [x] gcloud auth configure-docker asia-east1-docker.pkg.dev
│  ├─ [x] Artifact Registry repo 建立
│  ├─ [x] Service Account 建立
│  └─ [x] IAM roles/datastore.user 綁定
│
├─ [x] Step 1: docker build
├─ [x] Step 2: docker push
├─ [x] Step 3: gcloud run deploy
└─ [ ] Step 4: 驗證 /health endpoint
```

---

## 部署記錄

<!-- 每次部署在下方新增一個 section，格式如下 -->

---

### Deploy #1

**日期：** 2026-04-17
**部署人：** evan
**Image tag：** `latest`

#### 執行步驟結果

```text
Step 0: 前置準備
├─ gcloud auth login             ✅
├─ configure-docker              ✅
├─ artifacts repo create         ✅
├─ SA 建立                        ✅
└─ IAM 綁定                       ✅

Step 1: docker build
├─ 指令：docker build -f deployee/cloud_run/Dockerfile -t asia-east1-docker.pkg.dev/dailo-467502/docker-repo/internal-ai-copilot:latest .
├─ 結果：✅  (63.9s, FINISHED)
└─ 備註：首次 build，golang:1.25-alpine + alpine:3.21 layers 全部下載

Step 2: docker push
├─ 指令：docker push asia-east1-docker.pkg.dev/dailo-467502/docker-repo/internal-ai-copilot:latest
├─ 結果：✅
└─ digest: sha256:296a83157498e7b5587d5397dcae9b24c7dbfab3a3bfc3f5b8c3f95579033518

Step 3: gcloud run deploy
├─ 結果：✅  revision: internal-ai-copilot-00002-zpz
├─ Service URL：https://internal-ai-copilot-368821702422.asia-east1.run.app
└─ 備註：100% traffic routing 完成

Step 4: 驗證
├─ curl /health → 待驗證
└─ gRPC LineTaskConsult 測試 → 待 LineBot 部署後測試
```

#### 遇到的問題

```text
問題 1
├─ 現象：docker build 失敗，無法連線 daemon
├─ 根因：Docker Desktop 未啟動
└─ 修正：開啟 Docker Desktop 後重跑
```

#### 部署後記錄

```text
Service URL: https://internal-ai-copilot-368821702422.asia-east1.run.app
gRPC addr (for LineBot): internal-ai-copilot-368821702422.asia-east1.run.app:443
Image digest: sha256:296a83157498e7b5587d5397dcae9b24c7dbfab3a3bfc3f5b8c3f95579033518
Revision: internal-ai-copilot-00002-zpz
```

---

<!-- 複製上方 ### Deploy #N section 新增下一次部署 -->
