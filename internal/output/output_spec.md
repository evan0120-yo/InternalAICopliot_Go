# Output Module Spec

## Purpose
這份文件是 output module 的規格文件，用來定義此模組的責任、邊界、對外行為、分層方式與實作限制。

此文件用於模組層級的討論與設計對齊；具體 scenario、acceptance criteria、測試對應應維護在對應的 BDD 文件。

## Overview
Output 負責將 AI 回應渲染成最終輸出格式。根據 builder 設定決定是否產生檔案，支援 markdown 和 xlsx 兩種格式。

LinkChat profile-analysis 這條線第一版預期為 text-only，因此對應 builder 應走 `includeFile=false` 的 policy；output module 仍保留 generic file-rendering 能力。

對應 Java：`com.citrus.internalaicopilot.output`

## Layering In This Module

```text
UseCase -> Service
```

renderer 是 service 層內的格式實作，不作為獨立架構層。

## Responsibilities
- 判斷是否需要產生檔案
- 解析輸出格式
- 渲染 markdown / xlsx
- 轉換渲染結果為 base64
- 組裝檔名

## Layer Responsibilities

### UseCase
- 作為對 builder 暴露的 render 入口
- 對應主要 output 用例測試

### Service
- output policy
- renderer selection
- file encode

## Rendering Notes
- `includeFile=false` -> no file
- `includeFile=true` -> request output format or builder default
- xlsx renderer 先嘗試 markdown table，失敗則逐行輸出
