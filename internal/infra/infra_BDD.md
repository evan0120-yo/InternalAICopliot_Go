# Infra BDD Spec

## Purpose
這份文件定義 infra module 目前應滿足的共用基礎設施行為規格。內容以現有 code 與測試為基準。

## Actors
- 所有 module：透過 infra 取得 config、store、error surface 與 HTTP envelope
- HTTP handlers：使用 `WriteJSON`、`WriteError`、`DecodeJSONStrict`

## Scenario Group: Business Errors
- Given任意已知 business error
  When `AsBusinessError` 被呼叫
  Then 應保留原本的 `code`、`message` 與 `HTTPStatus`

- Given非 business error
  When `AsBusinessError` 被呼叫
  Then 應轉成 `INTERNAL_SERVER_ERROR`，且 message 必須被 generic masking

## Scenario Group: HTTP Envelope
- Given handler 要回傳成功結果
  When `WriteJSON` 執行
  Then 應回傳 `application/json; charset=utf-8` 並使用 `{ success, data }` envelope

- Given handler 要回傳錯誤
  When `WriteError` 執行
  Then 應回傳 `{ success: false, error: { code, message } }` envelope，HTTP status 由 business error 決定

## Scenario Group: Strict JSON Decode
- Given request body 超過限制
  When `DecodeJSONStrict` 執行
  Then 應回傳 `REQUEST_BODY_TOO_LARGE`

- Given request body 為空
  When `DecodeJSONStrict` 執行
  Then 應回傳 `INVALID_JSON`

- Given request body 不是合法 JSON 或含未知欄位或多個 JSON 物件
  When `DecodeJSONStrict` 執行
  Then 應回傳 `INVALID_JSON`

## Scenario Group: Store Seed And Reads
- Given Firestore emulator 內 apps/builders/templates collections 為空
  When `NewStore` 或 `NewStoreWithOptions` 建立成功且資料為空
  Then 應載入 `DefaultSeedData`

- Given local/dev bootstrap 明確要求 reset and seed
  When app 啟動前執行 infra bootstrap
  Then 系統應先清除既有 Firestore 開發資料，再重新載入 `DefaultSeedData`

- Given store 需要驗證 external app 存取
  When `AppByIDContext` 執行
  Then 應依 `appId` 讀出對應的 app 授權設定

- Given local/dev bootstrap 未開啟
  When app 啟動
  Then 系統不應主動清除既有資料

- Given多個 goroutine 同時讀取 builder、source、rag
  When context 未取消
  Then store 讀取應保持穩定，不因並發讀取產生錯誤

- Given context 已取消
  When `BuilderByIDContext`、`SourcesByBuilderIDContext` 或 `RagsBySourceIDContext` 執行
  Then 應尊重 context cancellation

- Given source document 帶 optional `moduleKey`
  When infra store 讀寫 builder graph
  Then 應保留該欄位，不在 infra 層改變它的業務語意

- Given app-aware prompt strategy 需要 strategy metadata
  When repository 讀取 `appPromptConfigs`
  Then 應可依 `appId` 取得對應的 prompt strategy 設定

- Given LinkChat strategy 需要 canonical key 可查找的 source graph
  When repository 讀寫 source document 的 `matchKey` 與 `sourceIds[]`
  Then 應原樣保留 lookup key 與 child source 順序，不在 infra 層自行改寫

## Scenario Group: Graph And Template Persistence
- Given builder graph replace 發生
  When `ReplaceBuilderGraph` 執行
  Then 現有非 system source 與其 rags 應被替換，system source 與其 rags 必須保留

- Given template 被儲存
  When `SaveTemplate` 執行
  Then template 與其 template rags 應被同時保存，更新時舊 rags 應被替換

- Given template order 被重排
  When `ReorderTemplates` 執行
  Then被指定的 template IDs 應獲得新的 canonical orderNo

- Given template 被刪除
  When `DeleteTemplate` 執行
  Then template、template rags 與 sources 上 copied-from-template references 都應被清除，剩餘 templates 要重新編排 orderNo

## Code-Backed Tests
- `errors_test.go`
- `http_test.go`
- `store_test.go`

## Open Questions
- source-level `moduleKey` 欄位的 production code 尚未落地，這份文件先鎖定資料責任與業務邊界
