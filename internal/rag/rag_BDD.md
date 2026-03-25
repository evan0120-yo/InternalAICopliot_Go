# RAG BDD Spec

## Purpose
這份文件定義 rag module 目前應滿足的行為規格。內容以現有 code 為準。

## Actors
- builder consult usecase：需要根據 sourceID 載入 RAG supplements
- rag resolve usecase：作為 builder 對 rag 的穩定入口

## Scenario Group: Resolve By Source
- Given builder 傳入 sourceID
  When `ResolveUseCase.ResolveBySourceID` 執行
  Then 應委派給 `ResolveService.ResolveBySourceID`

- Given store 能查到該 source 底下的 rags
  When `ResolveService.ResolveBySourceID` 執行
  Then 應依 store 回傳排序後的 supplements 原樣返回

- Given某個 rag 的 `RetrievalMode` 不是 `full_context`
  When resolver 正規化結果
  Then 應將其改寫為 `full_context`

- Given context 已取消或 store 讀取失敗
  When resolver 執行
  Then 應將錯誤往上回傳，不在 rag module 吞掉錯誤

## Acceptance Notes
- rag module 目前沒有動態 retrieval 分派邏輯
- 對外可觀測行為是「依 sourceID 載入補充內容，並將 retrieval mode 正規化為 `full_context`」
- 哪些 source 會進入 rag resolve，由 builder 先依 `analysisModules` 與 source `moduleKey` 決定

## Code-Backed Tests
- 目前沒有獨立 rag 測試檔
- 行為主要由 `builder/consult_usecase.go`、`builder/assemble_service.go` 與 `infra.Store` 讀取路徑間接使用

## Open Questions
- vector search 與 external API retrieval 尚未落地
- 未來若出現多種 retrieval strategy，是否仍維持現在的 service 介面尚未定案
