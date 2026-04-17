# Internal AI Copilot Go Backend Code Review

---

# BLOCK 1: AI 對產品的想像

這個 Go backend 現在看起來像一個 internal AI orchestration core。

它不是為單一產品寫死的後端，而是把多種入口收進同一套骨架：
- public HTTP
- external HTTP
- gRPC integration
- admin graph / templates

它現在最像的產品形態是：

```text
一個 builder-driven AI backend
├─ generic consult
├─ profile consult
├─ line task extraction
└─ admin config surface
```

主要使用者：
- internal frontend tester
- external app caller
- LinkChat 這類 profile caller
- 未來的 LineBot 類 caller
- admin 維護者

從 code 看得出的刻意選擇：
- HTTP 與 gRPC 沒走兩套業務邏輯，最後都收斂到 gatekeeper + builder consult。
- builder 現在靠 task builder factory 選 generic / profile / extract，不再把模式判斷塞在同一個大 switch。
- line task extraction 有自己的 contract，沒有硬塞進 ProfileConsult。
- promptguard 已經是獨立 module，而且真的接到 profile 主流程。
- output policy 與 AI orchestration 也被分開，避免 builder 自己處理檔案 render。

它目前不是：
- 不是多輪 chat session backend。
- 不是 vector-heavy RAG 平台。
- 不是已經有完整 admin auth 的後台。
- 不是已經 fully metadata-driven 的 UI/backoffice platform。

---

# BLOCK 2: 讀者模式

## A. 啟動後，整個骨架怎麼接起來

服務啟動時會先把 store、AI、builder、guard 都接好，再同時開 HTTP 與 gRPC。

```text
main
└─ app.New
   ├─ store
   ├─ rag
   ├─ aiclient
   ├─ output
   ├─ builder
   ├─ promptguard
   ├─ gatekeeper
   └─ register HTTP + gRPC
```

這表示外面看到的是兩種 transport，但裡面的業務骨架是共用的。

> 注意:
> admin route 目前沒有額外 auth gate。

## B. 不管從 HTTP 還是 gRPC 進來，generic consult 都走同一條主幹

generic consult 的共享核心很清楚：

```text
request 進來
└─ gatekeeper validate
   └─ builder consult
      ├─ load builder + sources
      ├─ task builder factory
      ├─ resolve rag
      ├─ assemble prompt
      ├─ aiclient analyze
      └─ output render
```

這條路徑有幾個固定關卡：
- builder 要存在且 active
- outputFormat 要合法
- attachments 要通過限制
- sources / rags 不能缺到無法執行

最後會不會產檔，不是 builder 自己決定，而是再過一次 output policy。

> 注意:
> generic consult、profile consult、extract consult 共用同一個 builder consult 主幹，只是 task builder 與 response contract 不同。

## C. Profile consult 現在已經是獨立模式，而且 promptguard 真的會先攔

profile consult 跟 generic consult 最大的差別，不是 transport，而是它會先把 structured profile 正規化，再決定是否要先跑 promptguard。

```text
profile request
└─ gatekeeper validate
   ├─ normalize subjectProfile
   ├─ shouldRunPromptGuard?
   └─ builder consult with profile mode
```

promptguard 目前只在特定條件下啟動：

```text
builderCode = linkchat-astrology
└─ userText 或 intentText 任一非空
   └─ promptguard
```

若 promptguard 判定 block，系統不會把 request 當成 validation 4xx，而是回一個正常 business response，表示這次回覆被取消。

> 注意:
> promptguard 現在不是 placeholder。它真的會做 rule match、score、必要時再升級到 LLM guard。

## D. Line task extraction 已經是獨立任務路線

line task extraction 現在有兩個入口：
- local/dev HTTP `POST /api/line-task-consult`
- external gRPC `LineTaskConsult`

兩條線最後都會進同一個 extract consult flow。

```text
line task request
└─ gatekeeper line-task validation
   └─ buildLineTaskCommand
      ├─ force live mode
      ├─ fill referenceTime/timeZone
      ├─ normalize supportedTaskTypes
      └─ builder consult with extract mode
```

這條線目前最重要的 current truth：
- `referenceTime` 與 `timeZone` 可以由 backend 自動補
- `supportedTaskTypes` 缺值時會預設 `["calendar"]`
- response 會被 parse 成 typed extraction result，而不是只回 raw string

> 注意:
> local/dev HTTP route 不承擔 external app auth；external gRPC route 則會驗 appId 與 allowed builders。

## E. Admin graph / templates 其實是另一條獨立工具線

除了 consult 以外，backend 還承接 admin graph 與 template library。

```text
admin handler
├─ graph load/save
└─ template CRUD
```

graph save 的語意不是 diff update，而是：

```text
保留 systemBlock
└─ 重寫非 systemBlock sources 與 rags
```

template library 則是獨立集合，create/update/delete 都有自己的 service 流程。

> 注意:
> template delete 目前不是 transaction；如果中途失敗，可能留下部分清理狀態。

## F. 現在最大的限制

```text
current limits
├─ admin auth 尚未落地
├─ app prompt config cache 沒有 active invalidation
├─ template delete 非 transaction
├─ source graph 沒有 cycle guard
└─ promptguard 目前只覆蓋特定 profile path
```

這幾件事是現在最容易影響成熟度判斷的地方。

---
