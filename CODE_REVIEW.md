# Internal AI Copilot — Go Backend Code Review

---

# ═══════════════════════════════════════════════
# BLOCK 1: AI 對這個產品的想像
# ═══════════════════════════════════════════════

這段是 AI 讀完所有程式碼後，對產品的「腦補」。
用途：讓讀者快速判斷 AI 有沒有搞懂這個系統在做什麼。如果這段歪了，後面的描述可能也歪。

## 它是什麼

一個**可設定的 AI 諮詢引擎**。核心概念是「Builder」——每個 Builder 代表一種 AI 諮詢任務（比如需求分析、測試案例生成）。管理員透過後台設定每個 Builder 的 prompt 結構（Sources + RAG 補充），使用者選一個 Builder 丟問題進去，系統把 prompt 拼好後，依執行模式決定：
- 正常模式：送 OpenAI / mock analyze
- preview mode：不呼叫 OpenAI，直接把完整 AI request preview 回給前端

不是聊天機器人。每次諮詢是獨立的一次性請求→回應，沒有對話歷史。

## 它的使用者是誰

- **前台使用者**：公司內部員工，透過 Web 介面選 Builder、輸入文字、上傳附件、取得 AI 分析結果
- **外部系統**：透過 X-App-Id 認證的第三方 App（限制只能用特定 Builder）
- **LinkChat**：主要透過 gRPC 的 ProfileConsult 串接；另外也有 public/local-dev 用的 HTTP `/api/profile-consult` 測試入口
- **管理員**：透過 Admin API 設定 Builder 的 prompt 結構和 Template 庫

## 規模想像

這是內部工具，不是面向消費者的高流量服務。我的猜測：

- **QPS**：個位數到兩位數，不會超過 100
- **Builder 數量**：目前 seed 資料只有 2 個，預期十幾到幾十個
- **單次請求耗時**：正常模式主要瓶頸在 OpenAI API 呼叫（write timeout 設 180 秒），不是 DB 讀寫；preview mode 會短路跳過這段
- **資料量**：Firestore 裡的 builders/sources/templates 數量都很小，所以很多地方用「全撈再 Go 層過濾」而不是 Firestore query

## 設計上的刻意選擇

1. **Clean Architecture 分層但很多層是空殼**：UseCase 層大量只是 `return s.service.Xxx(ctx, ...)` 一行轉發，預留未來擴充但目前沒邏輯
2. **HTTP + gRPC 雙協議**：HTTP 給前端和外部 App，gRPC 給 LinkChat。兩邊走同一個 UseCase 層，驗證邏輯完全一致
3. **Firestore 而非關聯式 DB**：文件結構是 `builders/{id}/sources/{id}/sourceRags/{id}` 的巢狀結構
4. **Mock 模式**：沒設 OPENAI_API_KEY 就自動走 mock，方便本地開發和測試
5. **整批替換不增量更新**：SaveGraph 是先全刪再全寫（在事務中），不做 diff

## 它不是什麼

- 不是 RAG 搜尋引擎——這裡的 "RAG" 是人工設定的靜態內容，不是向量搜尋
- 不是多輪對話系統——沒有 session、沒有記憶
- 不是多租戶 SaaS——沒有使用者認證（前端直接打，不帶 token），只有外部 App 的 X-App-Id

---

# ═══════════════════════════════════════════════
# BLOCK 2: 讀者模式
# ═══════════════════════════════════════════════

這段用「說人話」的方式描述每個 API 的行為。
目標：讀這段就等於讀了 code，但快 10 倍。

---

## A. 前台 / 外部 App：查 Builder 列表

**GET /api/builders**

前端開啟頁面第一件事就是呼叫這個，問系統「我能用哪些 AI 諮詢類型？」

系統從 Firestore 撈出所有 Builder，在 Go 層把 Active=false 的過濾掉，回傳一份精簡清單（只有 ID、代號、名稱、說明這些，不含 prompt 內容）。

沒有參數、沒有認證、沒有分頁。打就給你。

```
  使用者打 GET /api/builders
       │
       ▼
  從 Firestore 撈全部 builders
  → Go 層過濾掉 Active=false
  → 轉成精簡清單 (BuilderSummary)
       │
       ▼
  回傳 { success: true, data: [...] }
```

> 注意：過濾是 Go 層做的，不是 Firestore where 查詢。Builder 數量少所以沒問題，多了會有效能瓶頸。

> **外部 App 版本** (GET /api/external/builders)：多帶 `X-App-Id`，系統先驗 App 存在且啟用，再用 allowedBuilderIds 白名單過濾結果。

---

## B. 前台 / 外部 App：送出諮詢

**POST /api/consult**

這是系統的核心流程。使用者選好 Builder，打一些文字，可能附幾個檔案，按下送出。

入口格式是 multipart/form-data，帶這些欄位：
- `builderId`（必填，數字）
- `appId`（選填，只當 prompt strategy hint，不做 external auth）
- `text`（選填，使用者輸入的文字）
- `outputFormat`（選填，"markdown" 或 "xlsx"）
- `files`（選填，多檔上傳）

### 整體流程一覽

```
  使用者送出諮詢
       │
       ├── 1. 解析表單 ─── builderId 解不出來？→ 打回
       │
       ├── 2. 驗證 ─────── 通不過任何一關？→ 打回
       │                   (下面詳述每一關)
       │
       └── 3. 進入 Consult 引擎
             │
             ├── 載入 Builder 設定 + 所有 Sources（併發）
             │     └── sources 為空？→ 回 500
             │
             ├── Sources 裡有需要 RAG 補充的？→ 併發載入
             │     └── 標記要 RAG 但找不到？→ 回 500
             │
             ├── 組裝成完整 prompt（有嚴格順序）
             │
             ├── AI 執行模式判斷
             │     ├── preview mode？→ 直接回完整 request preview
             │     ├── 沒設 API Key？→ mock
             │     └── 其餘 → OpenAI
             │
             └── 看要不要產生檔案 → 回傳結果
```

### 驗證那一關到底在擋什麼

系統會依序檢查，任何一項不過就直接擋回去：

```
  ├── 解不出 client IP？→ 擋
  │
  ├── builderId 是 0？→ 擋
  ├── 這個 Builder 存不存在？→ 不存在就擋
  ├── 這個 Builder 是不是停用的？→ 停用就擋
  │
  ├── 有指定 outputFormat？
  │     └── 不是 markdown 也不是 xlsx？→ 擋
  │
  └── 有附件的話，逐個檢查：
        ├── 檔案數量超過上限？→ 擋（預設 10 個）
        ├── 單檔超過大小限制？→ 擋（預設 20MB）
        ├── 全部加起來超過總量？→ 擋（預設 50MB）
        └── 副檔名不在白名單？→ 擋
            (只接受 pdf/doc/docx/jpg/jpeg/png/webp/gif/bmp)
```

### Consult 引擎：從 prompt 到結果

這段邏輯被所有諮詢入口共用（前台、外部 App、gRPC 都走這裡），所以單獨拉出來講。

```
  通過驗證，進入 Consult 引擎
       │
       ▼
  ┌─ 第一步：併發載入 Builder + Sources ─┐
  │     └── sources 為空？→ 500          │
  └──────────────┬───────────────────────┘
                 │
                 ▼
  ┌─ 第二步：併發載入 RAG 補充 ──────────┐
  │     └── 標記要 RAG 但找不到？→ 500   │
  └──────────────┬───────────────────────┘
                 │
                 ▼
  ┌─ 第三步：組裝 prompt ───────────────────────────┐
  │                                                  │
  │  1. Framework Header                             │
  │     「你是 Internal AI Copilot 的內部 AI 顧問」  │
  │      帶上 builderId、builderCode、服務對象等     │
  │                                                  │
  │  2. [RAW_USER_TEXT]                              │
  │     使用者打的原始文字                            │
  │     (沒打就寫「用戶沒有額外需求」)               │
  │                                                  │
  │  3. [SUBJECT_PROFILE] ← 只有 Profile 模式才有   │
  │     LinkChat strategy 會先做理論翻譯             │
  │     再把最終語意片段直接組進這個 block           │
  │                                                  │
  │  4. [SOURCE-1], [SOURCE-2], ...                  │
  │     按順序排列的 prompt 片段                      │
  │     每個 Source 後面跟它的 RAG 補充               │
  │     (RAG 有 override 機制，下面說)               │
  │                                                  │
  │  5. [USER_INPUT]                                 │
  │     使用者的文字（如果還沒被 RAG override 吃掉） │
  │                                                  │
  │  6. [FRAMEWORK_TAIL]                             │
  │     強制 AI 回傳 JSON 格式的指示                  │
  │     加上安全檢查框架                              │
  └──────────────────────┬──────────────────────────┘
                         │
                         ▼
  ┌─ 第四步：呼叫 AI / preview ─────────────────────────────┐
  │     ├── AIPreviewMode=true → 不打 OpenAI               │
  │     │                        直接回 PROMPT_PREVIEW     │
  │     │                        response 是完整 request JSON │
  │     ├── 沒設 API Key → mock 模式                       │
  │     └── 其餘 → 真實 OpenAI                             │
  └──────────────┬─────────────────────────┘
                 │
                 ▼
  ┌─ 第五步：輸出渲染 ───────────────────────────────┐
  │     ├── response.Preview=true？→ 不產檔案        │
  │     ├── status=false？→ 不產檔案                 │
  │     ├── 沒開 IncludeFile？→ 不產檔案             │
  │     ├── markdown → 直接轉 bytes                  │
  │     └── xlsx → 解析表格 → 組裝                   │
  └──────────────┬─────────────────────────┘
                 │
                 ▼
            回傳結果
```

> **外部 App 版本** (POST /api/external/consult)：驗證前多一步 X-App-Id 驗證 + Builder 白名單檢查（不通過 → APP_BUILDER_FORBIDDEN），之後走同一個 Consult 引擎。

> **公開版補充**：`POST /api/consult` 現在可帶選填 `appId`。程式會把它一路傳到 builder，但因為 generic consult 沒有 `subjectProfile`，目前通常不會產生可見差異；這個欄位比較像是預留給 local/dev prompt strategy 測試。

---

## B-2. 前台本地測試：Profile Consult

**POST /api/profile-consult**

這條是 public/local-dev 的測試入口，讓你不用先接 gRPC，就能直接用 HTTP 丟 structured profile 測 prompt。

格式是 `application/json`，主要欄位：
- `appId`（選填，只當 prompt strategy hint）
- `builderId`（必填）
- `subjectProfile`（選填）
- `text`（選填）

其中 `subjectProfile` 現在的實際 shape 是：
- `subjectId`
- `analysisPayloads[]`
  - `analysisType`
  - `theoryVersion`（選填 metadata；有傳時不可為空白）
  - `payload`（任意 JSON 物件）

它和 gRPC `ProfileConsult` 的差別是：

1. **不做 external app auth**——`appId` 只是 strategy hint，不代表通過 LinkChat 權限驗證
2. **用途是本地/開發測 prompt**——如果直接暴露到 production，任何人都能帶任意 `appId` 走對應 strategy
3. **下游還是走同一套 profile consult flow**——會做同樣的 `subjectProfile / analysisPayloads` 驗證，再進 builder

所以它不是正式整合入口，比較像「給你先寫 prompt、先驗證 profile prompt 組裝結果的捷徑」。

---

## C. gRPC：ListBuilders / Consult

**rpc ListBuilders / rpc Consult**

gRPC 版本的查詢和諮詢。行為上跟 HTTP 版本完全一致，差別在：

1. **自動分流**：app_id 為空走公開路徑，有值走外部 App 路徑
2. **clientIP 解析不同**：優先用 request 裡帶的 client_ip，沒有就從 gRPC peer 的 Addr 取，都沒有就填 "grpc"
3. **Consult 的 File 傳輸方式不同**：gRPC 回應裡 file 是原始 bytes（backend 會把 base64 解開再傳）
4. **BuilderID 有截斷風險**：Go 內部是 int，gRPC proto 是 int32，超過 2^31 會截斷

補充：generic gRPC `Consult` 現在也會把 `app_id` 往下傳到 builder。和公開版 HTTP 一樣，因為 generic consult 沒有 `subjectProfile`，目前多半沒有可見差異，但欄位已保留。

---

## D. gRPC：ProfileConsult

**rpc ProfileConsult**

正式整合主路徑是 gRPC。現在雖然也有 public/local-dev 的 HTTP `/api/profile-consult`，但那條只是測試入口；真正給 LinkChat 走的還是這條。

跟普通 Consult 的根本差異：

1. **沒有附件和 outputFormat**——只有文字 + 結構化 profile
2. **Sources 會被過濾**——但現在是依 `subjectProfile.analysisPayloads` 推導 tag，不再靠 top-level `analysisModules`
3. **每個 analysis payload 可帶 theoryVersion**——目前只作 metadata 保留；LinkChat canonical-value lookup 不依賴它

### 驗證流程（比普通 Consult 多很多）

```
  ├── client IP 必須有
  ├── Builder 必須存在且啟用
  │
  ├── subjectProfile 如果有傳：
  │     ├── subjectId + analysisPayloads 都空 → 當沒傳
  │     ├── 有 analysisPayloads 但沒 subjectId → 擋
  │     └── 每個 analysis payload：
  │           ├── analysisType：trim → lowercase → 符合 ^[a-z0-9][a-z0-9_-]*$
  │           ├── analysisType 不能是 "common"
  │           ├── analysisType 不能重複
  │           ├── theoryVersion 如果有給，空白就擋
  │           └── theoryVersion 沒給也允許進 builder
  │
  └── text 空 + profile 空 → 擋
```

### 進入 Consult 引擎後的差異

跟普通模式比，Profile 模式在第一步和第三步有差異：

**第一步之後多了 Source 過濾**：

```
  每個 Source 的 moduleKey：
       │
       ├── moduleKey 格式非法？→ 500 錯誤（有測試覆蓋）
       │
       ├── moduleKey 為空 (包含 "common" 折疊成空)
       │     → 保留（通用 source，所有模組都吃得到）
       │
       ├── default strategy：
       │     取 analysisPayloads[].analysisType 正規化後的 tags
       │     moduleKey 在 tags 裡？→ 保留
       │
       ├── LinkChat strategy：
       │     先依 analysisType 挑 renderer
       │     renderer.SourceTags() 產生 tags
       │     moduleKey 在 tags 裡？→ 保留
       │
       └── 不在？→ 過濾掉

  過濾完都沒了？→ 500 錯誤
```

**第三步 prompt 裡多了 app-aware profile/context block**：

- default strategy：只產生 `[SUBJECT_PROFILE]`，把 `analysisPayloads` flatten 成 deterministic block
- LinkChat strategy：
  - 目前只支援 `astrology` 與 `mbti` 兩種 analysisType；其他值會直接報 `UNSUPPORTED_ANALYSIS_TYPE`
  - `astrology` 會直接把 payload 裡的 canonical value 對 `fragment source.matchKey`
  - 命中的 fragment source 會依 `sourceIds[]` 填入順序繼續展開 child sources
  - AI 最後只看到 source graph 組好的最終語意，不會看到 raw theory 詞，也不會看到 Internal private code
  - `mbti` 目前直接 raw render，不做 LinkChat-specific fragment lookup

所以現在同一個 ProfileConsult 裡，理論上可以混用：
- 已完成 composable source lookup 的 analysis payload（例如 astrology）
- 未做 app-specific 組裝的 analysis payload（例如 mbti）

這是目前 code 的真實行為，不代表最後產品規則一定會這樣定。

### 回應的重要限制

ProfileConsult 的 gRPC 回應**只回 status、statusAns、response 三個欄位**。就算 Builder 設了 IncludeFile=true，backend 內部照樣會跑完整的輸出渲染流程（包括產生檔案、base64 編碼），但 grpcapi 那層只取三個欄位回去，檔案被丟掉。

這是 v1 刻意的設計（ProfileConsultResponse 的 proto 沒有 file 欄位），不是 bug，但代表有不必要的渲染開銷。

---

## E. Admin：看 Builder 的 Prompt 結構 (Graph)

**GET /api/admin/builders/{builderId}/graph**

管理員要編輯某個 Builder 的 prompt 結構，先呼叫這個 API 把完整設定讀出來。

回傳的「graph」包含：
- Builder 本身的設定（名稱、代號、是否出檔案等）
- 所有 Sources（按順序），每個 Source 底下的 RAG 補充也一起帶出來

```
  解析 URL 裡的 builderId
  → 數字解析失敗？→ 擋
  → Builder 不存在？→ 404
  → 一層一層讀：builder → sources → 每個 source 的 rags
  → 組裝成完整回應

  呼叫鏈：AdminHandler → GraphUseCase → GraphService → QueryService → Store
```

> 注意：所有讀出來的 rag 的 retrievalMode 都會被強制設成 "full_context"。不管 DB 裡存什麼，出去一律是這個值。

---

## F. Admin：更新 Builder 的 Prompt 結構 (Graph)

**PUT /api/admin/builders/{builderId}/graph**

這是最複雜的 API。管理員修改完 prompt 結構後，整包送回來覆蓋。

### 整體流程

```
  收到 JSON body
       │
       ├── body 超過 1MB？→ 擋
       ├── JSON 有未知欄位？→ 擋
       ├── Builder 不存在？→ 404
       │
       ├── 合併 Builder 欄位（部分更新）
       │     只有送了的欄位才會改，沒送的維持原樣
       │     但 builderCode 改了要查重
       │     name 不能是空的
       │
       ├── 先存 Builder
       │
       ├── 正規化所有 Sources
       │     ├── systemBlock=true 的 source 完全跳過不動
       │     ├── 重新排序（按 orderNo，nil 排最後）
       │     ├── 重新分配 orderNo (1, 2, 3...)
       │     ├── 每個 source 的 moduleKey 正規化
       │     │   （"common" 折疊成空字串）
       │     └── 每個 RAG：ragType 必填、只接受 full_context
       │
       └── Firestore 事務：整批替換
             1. 讀取計數器（用來分配新 ID）
             2. 讀出所有現有的非 system sources + 它們的 rags
             3. 全部刪掉
             4. 寫入新的（分配真實 sourceId 和 ragId）
             5. 更新計數器
             → 完成後重新讀一次回傳最新狀態
```

> 注意：systemBlock source 在整個過程中被跳過。不會被刪、不會被重寫、不會被重新排序。

> 注意：有個 legacy 相容邏輯——如果 request.Sources 是空的，系統會嘗試從 request.AiAgent[].Source 裡取（舊版 Java 的 payload 格式）。

---

## G. Admin：查 Builder 可用的 Template

**GET /api/admin/builders/{builderId}/templates**

回傳這個 Builder「能看到」的 Template 清單。Template 有 GroupKey 機制：

```
  Template 的 GroupKey 是 null？
       └── 所有 Builder 都能看到它

  Template 的 GroupKey 有值？
       └── 只有 Builder 的 GroupKey 跟它一樣才看得到
```

另外只回 Active=true 的 Template。

呼叫鏈：AdminHandler → TemplateUseCase → TemplateService → QueryService → Store

---

## H. Admin：查全部 Template

**GET /api/admin/templates**

跟上面不同，這個回傳所有 Template，不做 GroupKey 過濾也不管 Active 狀態。管理員要看全貌用這個。

呼叫鏈：AdminHandler → TemplateUseCase → TemplateService → QueryService → Store

---

## I. Admin：新建 Template

**POST /api/admin/templates**

建一個新的可重用 Template。

```
  收到 JSON body
       │
       ├── templateKey 必填，不能跟現有的重複
       ├── name 必填
       ├── orderNo 必須正數（沒給就排最後）
       │
       ├── RAG 正規化（跟 graph 的 RAG 邏輯類似）
       │     ragType 必填、只接受 full_context
       │
       ├── Firestore 事務：
       │     分配 templateId、寫入 template + rags
       │
       └── 重新排序所有 template 的 orderNo
           → 回傳新建的 template
```

---

## J. Admin：更新 Template

**PUT /api/admin/templates/{templateId}**

跟新建類似，但：
- 會先確認 templateId 存在（不存在 → 404）
- 保留原本的 orderNo 和 active（除非 request 裡有明確指定）
- Rags 是整批替換（先刪後寫）

> 注意：UpdateTemplate 開頭先查一次 TemplateByIDContext，normalizeAndPrepareTemplate 裡面又查一次，同一個 template 讀了兩次 Firestore。

---

## K. Admin：刪除 Template

**DELETE /api/admin/templates/{templateId}**

刪除 Template 並清除所有引用。

```
  Template 不存在？→ 404
       │
       └── 開始刪除（注意：不在事務中）
             1. 刪除 template 底下的所有 templateRags
             2. 刪除 template 本身
             3. 掃描所有 builders 的所有 sources：
                如果 source 的 copiedFromTemplateID == 這個 templateId
                → 清除 5 個 copiedFromTemplate* 欄位
             4. 重新排序剩餘 templates 的 orderNo
```

> 注意：這整個流程不在 Firestore 事務中。如果中途失敗（比如步驟 3 掃到一半掛了），會留下不一致的狀態。

> 注意：步驟 3 會掃描「所有 builders 的所有 sources」。Builder 數量多時效能會很差。

---

## L. 系統啟動

```
  main()
       │
       ├── 讀環境變數 → Config
       ├── 初始化 Firestore
       │     ├── ResetOnStart=true？→ 清空重建 + seed
       │     └── SeedWhenEmpty=true？→ 空了才 seed
       │     → ensureMetadata：重算所有計數器 + 重建 sourceLookup
       │
       ├── 組裝所有 Service/UseCase/Handler（手動 DI）
       ├── 註冊 HTTP 路由 + 中介軟體
       │     withPanicRecovery → withCORS → Router
       │
       ├── 啟動 HTTP（預設 :8082）
       ├── 啟動 gRPC（預設 :9091）
       │
       └── 監聽 SIGTERM/SIGINT
           → 10 秒 graceful shutdown
           → HTTP Shutdown + gRPC GracefulStop
```

---

## M. 狀態與開關

這個系統沒有傳統的狀態機，但有幾個重要開關：

| 開關 | 影響 | 怎麼改 |
|------|------|--------|
| Builder.Active | =false 時不出現在列表、Consult 會被擋 | Admin PUT graph |
| App.Active | =false 時外部 App 打不進來 | 目前沒有 API 可改，只靠 seed |
| Template.Active | =false 時不出現在 Builder 的 template 列表（但 ListAll 看得到） | Admin PUT template |
| ConsultBusinessResponse.Status | AI 回 false 時不會觸發檔案渲染 | AI 自己決定 |
| Source.ModuleKey | 控制 Profile Consult 時哪些 source 會被載入 | Admin PUT graph |

---

# ═══════════════════════════════════════════════
# BLOCK 3: 技術補充
# ═══════════════════════════════════════════════

這段按 Block 2 的章節順序，提供完整的技術細節。
用途：AI 或開發者需要查具體錯誤碼、函式名稱、欄位定義時來這裡找。

---

## A. GET /api/builders + /api/external/builders 技術補充

**呼叫鏈**
```
gatekeeper.Handler.listBuilders
  → gatekeeper.UseCase.ListBuilders
    → builder.QueryService.ListActiveBuilders
      → infra.Store.ActiveBuildersContext
        → infra.Store.BuildersContext (全撈)
        → Go 層 filter Active==true
      → 轉換 BuilderConfig → BuilderSummary
  → infra.WriteJSON (200)
```

**回傳欄位**
```
BuilderSummary {
  builderId           int
  builderCode         string
  groupKey            *string (omitempty)
  groupLabel          string
  name                string
  description         string
  includeFile         bool
  defaultOutputFormat *string (omitempty)
}
```

**排序規則**：SortByOrderThenID → 先按 orderNo 再按 builderID（此 API 中 orderNo 固定 0，所以實際按 builderID）

**外部 App 版本呼叫鏈**
```
gatekeeper.Handler.listExternalBuilders
  → gatekeeper.UseCase.ListExternalBuilders
    → gatekeeper.GuardService.ValidateExternalApp
    → builder.QueryService.ListActiveBuilders
    → appAllowsBuilder (逐個比對 app.AllowedBuilderIDs)
  → infra.WriteJSON (200)
```

**外部 App 版本錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| APP_ID_MISSING | 400 | X-App-Id trim 後為空 |
| APP_NOT_FOUND | 400 | App 不存在 |
| APP_INACTIVE | 403 | App Active=false |

---

## B. POST /api/consult + /api/external/consult 技術補充

**呼叫鏈**
```
gatekeeper.Handler.consult
  → parseConsultMultipart (解析 multipart)
  → gatekeeper.GuardService.ResolveClientIP
  → gatekeeper.UseCase.Consult
    → gatekeeper.GuardService.ValidateConsult
      → validateActiveBuilder
      → ParseOutputFormat
      → 檔案驗證 (逐個)
    → builder.ConsultUseCase.Consult (ConsultModeGeneric)
      → Stage 1: WaitGroup (builder + sources 併發)
      → Stage 2: RAG WaitGroup (ragUseCase.ResolveBySourceID 併發)
        → rag.ResolveUseCase → rag.ResolveService → Store.RagsBySourceIDContext
        → 強制 retrievalMode = "full_context"
      → Stage 3: AssembleService.AssemblePrompt
      → Stage 4: aiclient.AnalyzeUseCase.Analyze
        → aiclient.AnalyzeService.Analyze
          → AIPreviewMode=true ? 直接回 preview JSON
          → 否則再走 mock / OpenAI
      → Stage 5: output.RenderUseCase.Render
        → output.RenderService.Render
          → response.Preview=true ? 跳過 file render
  → infra.WriteJSON (200)
```

補充：
- `POST /api/consult` 會讀 form 裡的 `appId`
- 這個 `appId` 會一路帶進 `builder.ConsultCommand.AppID`
- 但 generic consult 沒有 `subjectProfile`，所以目前通常只是在 strategy dispatch 上「走過流程但不產生內容」
- `INTERNAL_AI_COPILOT_AI_PREVIEW_MODE=true` 時，這條路徑不會打 OpenAI，而是回 `PROMPT_PREVIEW`
- preview `response` 內容是完整 AI request JSON：
  - `model`
  - `instructions`
  - `text.format.json_schema`
  - `input[0].content[]`
  - 附件只會列本地摘要，不會有真實 `file_id`

**錯誤碼清單（依檢查順序）**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| INVALID_MULTIPART | 400 | 不是 multipart/form-data |
| BUILDER_ID_MISSING | 400 | builderId 空或非數字 |
| FILE_READ_FAILED | 400 | 上傳檔案讀取失敗 |
| CLIENT_IP_MISSING | 400 | clientIP 為空 |
| BUILDER_ID_MISSING | 400 | builderID == 0 |
| BUILDER_NOT_FOUND | 400 | Builder 不存在 |
| BUILDER_INACTIVE | 403 | Builder Active=false |
| UNSUPPORTED_OUTPUT_FORMAT | 400 | 不是 markdown/xlsx |
| FILE_COUNT_EXCEEDED | 400 | 檔案數 > ConsultMaxFiles (預設 10) |
| FILE_SIZE_EXCEEDED | 400 | 單檔 > ConsultMaxFileSize (預設 20MB) |
| FILE_TOTAL_SIZE_EXCEEDED | 400 | 總量 > ConsultMaxTotalSize (預設 50MB) |
| UNSUPPORTED_FILE_TYPE | 400 | 副檔名不在白名單 |
| REQUEST_CANCELLED | 499 | context 取消或逾時 |
| SOURCE_ENTRIES_NOT_FOUND | 500 | sources 為空 |
| RAG_SUPPLEMENTS_NOT_FOUND | 500 | 標記需要 RAG 但找不到 |
| ATTACHMENT_UPLOAD_FAILED | 502 | 附件上傳 OpenAI 失敗 |
| ATTACHMENT_UPLOAD_REJECTED | 502 | OpenAI 拒絕附件 (4xx) |
| OPENAI_ANALYSIS_FAILED | 502 | OpenAI 分析失敗 |
| OPENAI_EMPTY_OUTPUT | 502 | OpenAI 回應無內容 |
| BUILDER_DEFAULT_OUTPUT_FORMAT_MISSING | 500 | Builder 需要出檔但沒設 defaultOutputFormat |
| BUILDER_DEFAULT_OUTPUT_FORMAT_INVALID | 500 | Builder 的 defaultOutputFormat 無效 |

**AIPreviewMode 行為**
```
1. 優先權最高：比 mock / OpenAI 都早判斷
2. 直接回 infra.ConsultBusinessResponse
   - status=true
   - statusAns="PROMPT_PREVIEW"
   - response=完整 request preview JSON
3. response.File 不會產生
4. Preview 是 internal flag（json:"-"），不會多吐一個前端欄位
5. preview 只原樣保留 builder 已組好的 instructions
   - 如果 builder 已做 composable source 組裝，preview 看到的是最終語意 prompt
   - 不會額外補出舊版 THEORY_CODEBOOK
```

**Prompt injection 檢查關鍵字 (mock 模式)**
```
英文: "ignore previous", "ignore all previous", "system prompt",
      "override instruction", "forget the rules"
中文: "忽略前面", "覆寫規則", "越權"
```

**ResolveClientIP 優先序**
```
1. X-Forwarded-For header (取第一個逗號前的值)
2. X-Real-IP header
3. RemoteAddr (SplitHostPort)
4. 原始 RemoteAddr
```

**OutputFormat 白名單**
```
"markdown" → OutputFormatMarkdown
"xlsx"     → OutputFormatXLSX
(大小寫不敏感，會 trim)
```

**檔案副檔名白名單**
```
pdf, doc, docx, jpg, jpeg, png, webp, gif, bmp
```

**XLSX 渲染邏輯**
```
parseMarkdownTable(response):
  找 | header | ... | + | --- | ... | + | data | ... | 的結構
  → 找到: cases sheet (表頭+資料) + summary sheet (表格外的文字)
  → 沒找到: consult sheet (逐行寫入)
buildXLSXWorkbook: 手動組裝 OOXML zip (不用第三方庫)
```

**檔案命名**
```
{filePrefix}-consult.{ext}
filePrefix 為空時: builder-{builderId}
```

**外部 App 版本呼叫鏈**
```
gatekeeper.Handler.externalConsult
  → parseConsultMultipart
  → gatekeeper.UseCase.ExternalConsult
    → gatekeeper.GuardService.ValidateExternalConsult
      → ValidateExternalApp (App 驗證)
      → ValidateConsult (同公開版)
      → appAllowsBuilder 檢查
    → builder.ConsultUseCase.Consult (ConsultModeGeneric)
  → infra.WriteJSON (200)
```

**外部 App 版本額外錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| APP_ID_MISSING | 400 | X-App-Id 為空 |
| APP_NOT_FOUND | 400 | App 不存在 |
| APP_INACTIVE | 403 | App 已停用 |
| APP_BUILDER_FORBIDDEN | 403 | Builder 不在 App 白名單 |

---

## B-2. POST /api/profile-consult 技術補充

**呼叫鏈**
```
gatekeeper.Handler.profileConsult
  → json.Decoder + DisallowUnknownFields
  → profileConsultRequest.toSubjectProfile
  → gatekeeper.GuardService.ResolveClientIP
  → gatekeeper.UseCase.PublicProfileConsult
    → gatekeeper.GuardService.ValidateProfileConsult
    → builder.ConsultUseCase.Consult (ConsultModeProfile)
  → infra.WriteJSON (200)
```

**這條路徑的特性**
```
- appId 是 optional，而且只當 prompt strategy hint
- 不走 ValidateExternalApp
- 不做 appAllowsBuilder 白名單檢查
- 用途是本地 / 開發階段直接測 profile prompt
```

**錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| INVALID_JSON | 400 | body 不是合法 JSON 或帶未知欄位 |
| THEORY_VERSION_MISSING | 400 | theoryVersion 有傳但 trim 後是空字串 |
| 其餘 ProfileConsult 驗證錯誤 | 同 gRPC | 共用 ValidateProfileConsult |

---

## C. gRPC ListBuilders / Consult 技術補充

**ListBuilders 呼叫鏈**
```
grpcapi.Service.ListBuilders
  → appID 為空? → useCase.ListBuilders (公開)
  → appID 有值? → useCase.ListExternalBuilders (app 驗證)
  → []BuilderSummary → []*grpcpb.BuilderSummary
     BuilderID: int → int32 (截斷風險)
     GroupKey, DefaultOutputFormat: 有值才設
```

**Consult 呼叫鏈**
```
grpcapi.Service.Consult
  → 轉換 attachments: grpcpb.Attachment → infra.Attachment
  → resolveClientIP(ctx, request.ClientIp)
  → appID 為空? → useCase.Consult (公開)
  → appID 有值? → useCase.ExternalConsult (app 驗證)
  → 回應轉換:
     status, statusAns, response: 直傳
     File 存在時:
       base64.StdEncoding.DecodeString(File.Base64)
       解碼失敗 → INVALID_FILE_PAYLOAD (500)
       成功 → grpcpb.FilePayload (fileName, contentType, data bytes)
```

**gRPC resolveClientIP 優先序**
```
1. request.ClientIp (trim 後非空就用)
2. peer.FromContext → Addr → SplitHostPort
3. peer.Addr 直接用
4. fallback "grpc"
```

**gRPC 錯誤碼映射 (asGRPCError)**
```
HTTP 400 → codes.InvalidArgument
HTTP 401 → codes.Unauthenticated
HTTP 403 → codes.PermissionDenied
HTTP 404 → codes.NotFound
HTTP 409 → codes.AlreadyExists
HTTP 413 → codes.ResourceExhausted
HTTP 429 → codes.ResourceExhausted
HTTP 499 → codes.Canceled
HTTP 500 → codes.Internal
HTTP 501 → codes.Unimplemented
HTTP 502/503 → codes.Unavailable
HTTP 504 → codes.DeadlineExceeded
```
錯誤額外帶 ErrorInfo.Reason = BusinessError.Code

---

## D. gRPC ProfileConsult 技術補充

**呼叫鏈**
```
grpcapi.Service.ProfileConsult
  → resolveClientIP
  → toSubjectProfile (grpcpb → builder.SubjectProfile 深拷貝)
     → theoryVersion 也會深拷貝
  → gatekeeper.UseCase.ProfileConsult
    → appID 為空? → GuardService.ValidateProfileConsult
    → appID 有值? → GuardService.ValidateExternalProfileConsult
      → ValidateExternalApp + ValidateProfileConsult + appAllowsBuilder
    → builder.ConsultUseCase.Consult (ConsultModeProfile)
      → AssembleService.FilterProfileSources
      → AssembleService.buildProfileContextBlock
         → resolveProfileContextStrategy
            → appId=""、查不到 config、config inactive、strategyKey="" → default
            → appPromptConfig.strategyKey=linkchat → LinkChat strategy
         → 需要時做 theory mapping / semantic translation 組裝
      → 其餘同 Generic
  → 回應只取 status/statusAns/response (丟棄 File)
```

**ValidateProfileConsult 錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| CLIENT_IP_MISSING | 400 | clientIP 為空 |
| BUILDER_ID_MISSING | 400 | builderID == 0 |
| BUILDER_NOT_FOUND | 400 | Builder 不存在 |
| BUILDER_INACTIVE | 403 | Builder 已停用 |
| INVALID_MODULE_KEY | 400 | analysisType 空或格式非法 |
| RESERVED_MODULE_KEY | 400 | analysisType 是 "common" |
| SUBJECT_ID_MISSING | 400 | 有 analysisPayloads 但沒 subjectId |
| DUPLICATE_ANALYSIS_PAYLOAD | 400 | analysisType 重複 |
| THEORY_VERSION_MISSING | 400 | theoryVersion 有傳但 trim 後為空 |
| PROFILE_INPUT_EMPTY | 400 | text + profile 全空 |
| APP_BUILDER_FORBIDDEN | 403 | Builder 不在 App 白名單 (external) |

**FilterProfileSources 錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| INVALID_SOURCE_MODULE_KEY | 500 | source 儲存的 moduleKey 格式非法 (有測試覆蓋) |
| SOURCE_ENTRIES_NOT_FOUND | 500 | 過濾後 sources 為空 |

**moduleKey 正規化規則**
```
NormalizeStoredModuleKey(raw):
  trim → lowercase
  "" 或 "common" → 回傳 ""
  不符合 ^[a-z0-9][a-z0-9_-]*$ → INVALID_MODULE_KEY

NormalizeAnalysisTypeKey(raw):
  trim → lowercase
  空值 → INVALID_MODULE_KEY
  "common" → RESERVED_MODULE_KEY
  不符合 ^[a-z0-9][a-z0-9_-]*$ → INVALID_MODULE_KEY
```

**SubjectProfile prompt 組裝格式**
```
## [SUBJECT_PROFILE]
subject: {subjectId}

### [analysis:{analysisType}]
theory_version: {theoryVersion}        // 只有 payload.TheoryVersion 非空且 includeTheoryVersion=true 時才會有
{payloadKey}: {value1}|{value2}|{value3}

(analysisPayloads 按 analysisType 排序，payload keys 按 key 排序)
(values 中的 \ 跳脫為 \\，| 跳脫為 \|)
```

**analysis payload flatten 規則**
```
payload map[string]any
  ├── key：trim → lowercase；空白/空字串 key 直接忽略
  ├── key 中的 space / "-" / "." → "_"
  ├── string：trim 後非空才保留
  ├── bool / number：轉字串
  ├── []string / []any：逐項展平
  ├── map[string]any：
  │     ├── 如果含 key 或 value 欄位 → 只取該欄位字串值
  │     │   其他欄位（例如 weightPercent）目前不進 prompt
  │     └── 否則遞迴展平成 parent_child_key
  └── 其他型別：先 json.Marshal；Marshal 失敗才報 INVALID_ANALYSIS_PAYLOAD
```

**LinkChat strategy 的額外 block**
```
## [SUBJECT_PROFILE]
subject: {subjectId}

### [analysis:astrology]
theory_version: astro-v1
主執行緒, 發展有好有壞, 主導做事方式和習慣, 以及思維output框架: 深層洞察
OS 內核, 主導思維底層邏輯, 包含思維intput架構及運算方式, 喝醉時會同時兼任主執行緒（依照喝醉狀況更多取代本來主執行緒）: 敏感共感
```

這段不是 AI 自己解碼得出的結果，而是 builder 先做：
```text
slotKey + canonical value
  ├─ primary source: matchKey = slotKey
  ├─ fragment source: matchKey = canonical value
  ├─ fragment source 若有 sourceIds[]，照陣列順序繼續展開 child sources
  └─ Internal 直接組成最終語意行
```

**app-aware strategy/runtime 錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| UNKNOWN_PROMPT_STRATEGY | 500 | appPromptConfigs 裡的 strategyKey 不認得 |
| SOURCE_FRAGMENT_NOT_FOUND | 500 | canonical value 找不到對應的 fragment source |
| SOURCE_REFERENCE_NOT_FOUND | 500 | fragment source 的 sourceIds[] 參照不存在 |
| UNSUPPORTED_ANALYSIS_TYPE | 400 | LinkChat strategy 不認得 analysisType |
| INVALID_ANALYSIS_PAYLOAD | 400 | payload 值無法展平，且 json.Marshal 也失敗 |

**目前的限制 / 刻意設計**
```
1. LinkChat astrology 現在會把 canonical value 直接展開成 source graph，放在 SUBJECT_PROFILE 內
2. appPromptConfig 有 process-local cache，但沒有 TTL / invalidation
3. Firestore 裡改了 strategy config，要重啟服務才保證吃到最新值
4. `theoryVersion` 現在只作 metadata 保留；有傳時不能是空字串，但不是 astrology 必填欄位
```

---

## E. GET /api/admin/builders/{builderId}/graph 技術補充

**呼叫鏈**
```
builder.AdminHandler.loadGraph
  → parseIntPathValue(r, "builderId")
  → builder.GraphUseCase.LoadGraph
    → builder.GraphService.LoadGraph
      → builder.QueryService.LoadGraph
        → store.BuilderByIDContext
        → store.SourcesByBuilderIDContext
           排序: orderNo → sourceId
        → foreach source: store.RagsBySourceIDContext
           排序: orderNo → ragId
           retrievalMode 強制 "full_context"
           moduleKey 空值 → JSON omit
           sourceType / matchKey 空值 → JSON omit
           sourceIds[] 原樣帶回
        → 組裝 BuilderGraphResponse
  → infra.WriteJSON (200)
```

**錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| BUILDER_ID_MISSING | 400 | 路徑值空或非數字 |
| BUILDER_NOT_FOUND | 404 | Builder 不存在 |

---

## F. PUT /api/admin/builders/{builderId}/graph 技術補充

**呼叫鏈**
```
builder.AdminHandler.saveGraph
  → parseIntPathValue
  → infra.DecodeJSONStrict (1MB limit, DisallowUnknownFields)
  → builder.GraphUseCase.SaveGraph
    → builder.GraphService.SaveGraph
      → store.BuilderByIDContext (確認存在)
      → mergeBuilder (部分更新)
      → store.SaveBuilder
      → extractSourceRequests (優先 Sources, fallback AiAgent)
      → normalizeGraphSources
      → store.ReplaceBuilderGraph (Firestore 事務)
      → QueryService.LoadGraph (重讀回傳)
```

**mergeBuilder 欄位對照表**

| 欄位 | nil 行為 | 驗證 |
|------|---------|------|
| builderCode | 不更新 | 空→BUILDER_FIELD_MISSING; 重複→BUILDER_CODE_DUPLICATE |
| groupLabel | 不更新 | 有值才覆蓋 |
| groupKey | 不更新 | 有值覆蓋; 都沒有就從 groupLabel 衍生 (小寫+非字母數字換"-") |
| name | 不更新 | 空→BUILDER_FIELD_MISSING |
| description | 不更新 | 直接覆蓋 (允許空) |
| includeFile | 不更新 | 直接覆蓋 |
| defaultOutputFormat | 不更新 | 空→設nil; 非空→ParseOutputFormat, 無效→UNSUPPORTED_OUTPUT_FORMAT |
| filePrefix | 不更新 | trim 後覆蓋 |
| active | 不更新 | 直接覆蓋 |

**normalizeGraphSources 規則**

1. systemBlock=true 的 source 直接跳過
2. 按 orderNo → 原始 index 排序 (orderNo nil 排最後)
3. foreach source:
   - orderNo 有提供但 ≤0 → SOURCE_ORDER_INVALID (400)
   - 重新分配 canonical orderNo (1, 2, 3...)
   - sourceID 設為負數佔位符 -(index+1)
   - moduleKey: NormalizeStoredModuleKey (common 折疊成空)
   - sourceType: 只接受 primary / fragment，其餘 → SOURCE_TYPE_INVALID (400)
   - matchKey: trim 後保留
   - sourceIds[]: 先保留 request 內的 source 參照，normalize 完再改寫成佔位 sourceID
   - NeedsRagSupplement = len(rag) > 0
   - 保留 template 引用欄位
4. foreach rag:
   - orderNo ≤0 → RAG_ORDER_INVALID (400)
   - ragType 必填 → RAG_TYPE_MISSING (400)
   - retrievalMode 只支援 "full_context" → RAG_RETRIEVAL_MODE_UNSUPPORTED (400)
   - title: 優先 request.Title，否則用 ragType
   - content: 優先 request.Content，否則 fallback request.Prompts (deprecated)
   - overridable: 預設 false

**ReplaceBuilderGraph Firestore 事務**
```
╔════════════════════════════════════════════════╗
║  RunTransaction                                ║
╠════════════════════════════════════════════════╣
║  1. 讀 _meta/counters (nextSourceId, nextRagId)║
║  2. 讀現有 sources (跳過 systemBlock)           ║
║     讀每個 source 底下的 sourceRags             ║
║  3. 刪除現有 (跳過 systemBlock):                ║
║     foreach source:                            ║
║       刪除所有 sourceRags                       ║
║       刪除 _sourceLookup/{sourceId}             ║
║       刪除 source 本身                          ║
║  4. 寫入新的:                                   ║
║     先為每個 placeholder source 預分配真實 ID     ║
║     foreach source:                            ║
║       source.SourceID = 預分配的新 ID            ║
║       source.SourceIDs[] 依預分配表重寫          ║
║       Set builders/{bid}/sources/{sid}          ║
║       Set _sourceLookup/{sid}                  ║
║       foreach rag (SourceID 匹配佔位符):        ║
║         counters.NextRagID++                   ║
║         rag.RagID = 新 ID                      ║
║         rag.SourceID = 真實 sourceID            ║
║         Set sourceRags/{ragId}                 ║
║  5. 回寫 _meta/counters                        ║
╚════════════════════════════════════════════════╝
```

**SaveGraph 完整錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| BUILDER_ID_MISSING | 400 | 路徑值問題 |
| INVALID_JSON | 400 | JSON 格式問題 |
| REQUEST_BODY_TOO_LARGE | 413 | Body 超過 1MB |
| BUILDER_NOT_FOUND | 404 | Builder 不存在 |
| BUILDER_FIELD_MISSING | 400 | builderCode 或 name 為空 |
| BUILDER_CODE_DUPLICATE | 400 | builderCode 跟別的 builder 撞了 |
| UNSUPPORTED_OUTPUT_FORMAT | 400 | defaultOutputFormat 無效 |
| SOURCE_ID_DUPLICATE | 400 | graph request 內 sourceId 重複 |
| INVALID_MODULE_KEY | 400 | source moduleKey 格式非法 |
| SOURCE_TYPE_INVALID | 400 | sourceType 不是 primary / fragment |
| SOURCE_ORDER_INVALID | 400 | source orderNo ≤ 0 |
| SOURCE_REFERENCE_NOT_FOUND | 400 | sourceIds[] 指到 request 內不存在的 source |
| RAG_ORDER_INVALID | 400 | rag orderNo ≤ 0 |
| RAG_TYPE_MISSING | 400 | ragType 為空 |
| RAG_RETRIEVAL_MODE_UNSUPPORTED | 400 | retrievalMode 不是 full_context |

---

## G. GET /api/admin/builders/{builderId}/templates 技術補充

**呼叫鏈**
```
builder.AdminHandler.listBuilderTemplates
  → parseIntPathValue
  → builder.TemplateUseCase.ListTemplatesByBuilder
    → builder.TemplateService.ListTemplatesByBuilder
      → builder.QueryService.ListTemplatesByBuilder
        → store.BuilderByIDContext
        → store.TemplatesContext (全撈)
        → 過濾: Active=false 跳過; GroupKey 不匹配跳過
        → foreach: store.TemplateRagsByTemplateIDContext
        → sortTemplates (orderNo → templateId)
        → toTemplateResponses (retrievalMode 強制 full_context)
```

**GroupKey 過濾邏輯**
```
template.GroupKey == nil → 通過 (對所有 builder 可見)
template.GroupKey != nil:
  builder.GroupKey == nil → 不通過
  *template.GroupKey != *builder.GroupKey → 不通過
  *template.GroupKey == *builder.GroupKey → 通過
```

---

## H. GET /api/admin/templates 技術補充

**呼叫鏈**
```
builder.AdminHandler.listAllTemplates
  → builder.TemplateUseCase.ListAllTemplates
    → builder.TemplateService.ListAllTemplates
      → builder.QueryService.ListAllTemplates
        → store.TemplatesContext (全撈，不過濾)
        → foreach: store.TemplateRagsByTemplateIDContext
        → sortTemplates
        → toTemplateResponses
```

---

## I. POST /api/admin/templates 技術補充

**呼叫鏈**
```
builder.AdminHandler.createTemplate
  → infra.DecodeJSONStrict
  → builder.TemplateUseCase.CreateTemplate
    → builder.TemplateService.CreateTemplate
      → normalizeAndPrepareTemplate (isCreate=true)
        → 驗證 templateKey, name, orderNo
        → TemplateByKeyContext (查重)
        → TemplatesContext (算 orderNo 預設值 = len+1)
        → normalizeTemplateRags
      → store.SaveTemplate (Firestore 事務)
      → reorderTemplateIDs + store.ReorderTemplates
      → templateResponseByID (透過 ListAllTemplates 查回來)
```

**SaveTemplate Firestore 事務**
```
╔════════════════════════════════════════════════╗
║  RunTransaction                                ║
╠════════════════════════════════════════════════╣
║  1. 讀 _meta/counters                          ║
║  2. templateID == 0 (新建):                    ║
║     counters.NextTemplateID++                  ║
║     template.TemplateID = 新 ID               ║
║  3. 讀現有 templateRags (全部)                  ║
║  4. Set template 到 templates/{id}             ║
║  5. 刪除所有現有 templateRags                   ║
║  6. 寫入新 rags:                               ║
║     foreach rag:                               ║
║       counters.NextTemplateRag++               ║
║       Set templateRags/{id}                    ║
║  7. 回寫 _meta/counters                       ║
╚════════════════════════════════════════════════╝
```

**CreateTemplate 錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| TEMPLATE_KEY_MISSING | 400 | templateKey 為空 |
| TEMPLATE_NAME_MISSING | 400 | name 為空 |
| TEMPLATE_ORDER_INVALID | 400 | orderNo ≤ 0 |
| TEMPLATE_KEY_DUPLICATE | 400 | templateKey 重複 |
| TEMPLATE_RAG_TYPE_MISSING | 400 | rag ragType 為空 |
| TEMPLATE_RAG_ORDER_INVALID | 400 | rag orderNo ≤ 0 |
| RAG_RETRIEVAL_MODE_UNSUPPORTED | 400 | retrievalMode 不是 full_context |

---

## J. PUT /api/admin/templates/{templateId} 技術補充

**呼叫鏈**
```
builder.AdminHandler.updateTemplate
  → parseInt64PathValue
  → infra.DecodeJSONStrict
  → builder.TemplateUseCase.UpdateTemplate
    → builder.TemplateService.UpdateTemplate
      → store.TemplateByIDContext (第一次讀，確認存在)
      → normalizeAndPrepareTemplate (isCreate=false)
        → store.TemplateByIDContext (第二次讀，取 existing)
        → 保留 existing.OrderNo 和 existing.Active
      → store.SaveTemplate (事務)
      → reorderTemplateIDs + store.ReorderTemplates
      → templateResponseByID
```

**額外錯誤碼**

| 錯誤碼 | HTTP | 觸發條件 |
|--------|------|---------|
| TEMPLATE_ID_MISSING | 400 | 路徑值問題 |
| TEMPLATE_NOT_FOUND | 404 | Template 不存在 |

---

## K. DELETE /api/admin/templates/{templateId} 技術補充

**呼叫鏈**
```
builder.AdminHandler.deleteTemplate
  → parseInt64PathValue
  → builder.TemplateUseCase.DeleteTemplate
    → builder.TemplateService.DeleteTemplate
      → store.TemplateByIDContext (確認存在)
      → store.DeleteTemplate (非事務)
        → 刪除 templateRags
        → 刪除 template
        → BuildersContext (全撈)
        → foreach builder: 讀 sources
          → foreach source: 檢查 copiedFromTemplateID
          → 匹配的清除 5 個 copiedFromTemplate* 欄位
            (用 firestore.Delete + MergeAll)
        → TemplatesContext → SortByOrderThenID
        → foreach: Update orderNo = index+1
```

**非事務操作的失敗點**
```
步驟 1 (刪 rags) 失敗 → rags 部分刪除
步驟 2 (刪 template) 失敗 → template 還在但部分 rags 已刪
步驟 3 (清引用) 失敗 → template 已刪但 sources 還引用著不存在的 template
步驟 4 (重排序) 失敗 → 其他 templates 的 orderNo 不連續
```

---

## L. 系統啟動 技術補充

**Config 環境變數與預設值**

| 設定 | 環境變數 | 預設值 |
|------|---------|--------|
| HTTP 位址 | INTERNAL_AI_COPILOT_ADDR | :8082 |
| gRPC 位址 | INTERNAL_AI_COPILOT_GRPC_ADDR | :9091 |
| Firestore 專案 | INTERNAL_AI_COPILOT_FIRESTORE_PROJECT_ID | dailo-467502 |
| Firestore 模擬器 | INTERNAL_AI_COPILOT_FIRESTORE_EMULATOR_HOST | localhost:8090 |
| 重啟清資料 | INTERNAL_AI_COPILOT_STORE_RESET_ON_START | false |
| CORS 白名單 | INTERNAL_AI_COPILOT_CORS_ALLOWED_ORIGINS | localhost:3000, 127.0.0.1:3000 |
| 最大檔案數 | INTERNAL_AI_COPILOT_CONSULT_MAX_FILES | 10 |
| 單檔大小限制 | INTERNAL_AI_COPILOT_CONSULT_MAX_FILE_SIZE_BYTES | 20MB |
| 總大小限制 | INTERNAL_AI_COPILOT_CONSULT_MAX_TOTAL_SIZE_BYTES | 50MB |
| Server read timeout | INTERNAL_AI_COPILOT_SERVER_READ_TIMEOUT | 10s |
| Server write timeout | INTERNAL_AI_COPILOT_SERVER_WRITE_TIMEOUT | 180s |
| OpenAI timeout | INTERNAL_AI_COPILOT_OPENAI_TIMEOUT | 120s |
| AI preview mode | INTERNAL_AI_COPILOT_AI_PREVIEW_MODE | false |
| OpenAI API Key | OPENAI_API_KEY | (空→mock) |
| OpenAI Base URL | OPENAI_BASE_URL | https://api.openai.com/v1 |
| AI 模型 | INTERNAL_AI_COPILOT_AI_MODEL | gpt-4o |

所有設定都支援 legacy 前綴 `REWARDBRIDGE_*` 作為 fallback。

**DI 組裝順序 (app.New)**
```
1. Store (Firestore)
2. rag.ResolveService → rag.ResolveUseCase
3. aiclient.AnalyzeService → aiclient.AnalyzeUseCase
4. output.RenderService → output.RenderUseCase
5. builder.QueryService
6. builder.AssembleService
7. builder.GraphService (依賴 Store + QueryService)
8. builder.TemplateService (依賴 Store + QueryService)
9. builder.ConsultUseCase (依賴 Store + RAG + AI + Output + Assemble)
10. builder.GraphUseCase (依賴 GraphService)
11. builder.TemplateUseCase (依賴 TemplateService)
12. builder.AdminHandler (依賴 GraphUseCase + TemplateUseCase)
13. gatekeeper.GuardService (依賴 Config + Store)
14. gatekeeper.UseCase (依賴 Guard + QueryService + ConsultUseCase)
15. gatekeeper.Handler (依賴 UseCase)
```

**HTTP 中介軟體鏈**
```
withPanicRecovery
  → defer recover() → log + INTERNAL_SERVER_ERROR (500)
withCORS
  → Origin 精確匹配 allowedOrigins 清單
  → 設定 CORS headers (Allow-Headers 含 X-App-Id)
  → OPTIONS → 204 直接回
Router (http.ServeMux)
```

**Firestore 持久化結構**
```
apps/{appId}                                      AppAccess
appPromptConfigs/{appId}                          AppPromptConfig
builders/{builderId}                              BuilderConfig
builders/{builderId}/sources/{sourceId}            Source
builders/{builderId}/sources/{sourceId}/sourceRags/{ragId}  RagSupplement
templates/{templateId}                            Template
templates/{templateId}/templateRags/{ragId}        TemplateRag
_meta/counters                                    {nextSourceId, nextRagId, nextTemplateId, nextTemplateRagId}
_sourceLookup/{sourceId}                          {sourceId, builderId}
```

**HTTP 回應信封格式**
```json
成功: { "success": true, "data": { ... } }
失敗: { "success": false, "error": { "code": "ERROR_CODE", "message": "..." } }
```
