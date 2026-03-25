# Output BDD Spec

## Purpose
這份文件定義 output module 目前應滿足的行為規格。內容以現有 code 與測試為基準。

## Actors
- builder consult usecase：將 AI business response 與 builder output policy 交給 output module
- output render usecase：對 builder 暴露 render 入口

## Scenario Group: Output Policy
- Given business response `status=false`
  When `RenderService.Render` 執行
  Then 應直接回傳原 response，且 `file` 必須為 `nil`

- Given builder `IncludeFile=false`
  When `RenderService.Render` 執行
  Then 應直接回傳原 response，且不產生檔案

- Given builder `IncludeFile=true` 且沒有 default output format
  When `RenderService.Render` 執行
  Then 應回傳 `BUILDER_DEFAULT_OUTPUT_FORMAT_MISSING`

- Given builder default output format 非 `markdown` / `xlsx`
  When `RenderService.Render` 執行
  Then 應回傳 `BUILDER_DEFAULT_OUTPUT_FORMAT_INVALID`

- Given request 指定 output format
  When `RenderService.Render` 執行
  Then request 指定值應覆蓋 builder default

## Scenario Group: Markdown Rendering
- Given resolved format 為 `markdown`
  When render 執行
  Then 應輸出 `text/markdown; charset=utf-8` 檔案，內容即為 AI response 原文

## Scenario Group: XLSX Rendering
- Given AI response 包含合法 markdown table
  When `renderXLSX` 執行
  Then 應產出至少一個 `cases` sheet，第一列放 builder name，後續列放 table headers 與 rows

- Given markdown table 之外還有其他非空摘要文字
  When `renderXLSX` 執行
  Then 應額外產出 `summary` sheet 保存摘要內容

- Given AI response 不是 markdown table
  When `renderXLSX` 執行
  Then 應回退成 `consult` sheet，以逐行方式輸出原始內容

## Scenario Group: File Payload
- Given render 成功
  When `RenderService.Render` 組裝最終回應
  Then 應將檔案 bytes base64 encode 後放進 `ConsultFilePayload`

- Given builder 沒有 `FilePrefix`
  When output module 建立檔名
  Then 應使用 `builder-{builderId}-consult.{ext}` 作為預設檔名

## Acceptance Notes
- LinkChat profile-analysis 第一版預期 builder `IncludeFile=false`，因此正常路徑應只回純文字 response

## Code-Backed Tests
- `service_test.go`
- `renderer_xlsx_test.go`

## Open Questions
- markdown renderer 目前幾乎是 passthrough，未來是否要補 frontmatter 或 metadata 尚未定案
- xlsx fallback 與 cases/summary 命名目前依現有前端需求而定，若前端契約改動需同步修文件與測試
