# Internal AI Copilot Go Backend

## 啟動前先知道

- Go 專案路徑：`D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go`
- 現在日常啟動只需要記 3 個環境變數：
  - `GEMINI_API_KEY`
  - `OPENAI_API_KEY`
  - `INTERNAL_AI_COPILOT_AI_PROFILE`
- `linkchat-astrology` 的 profile request 若 `text` 不為空，會先跑 `promptguard -> Gemma`。
- 所以即使主 AI 最後是 `preview`、`mock`、`openai`，只要要測 `linkchat-astrology + text`，通常都建議先把 `GEMINI_API_KEY` 設好。
- `INTERNAL_AI_COPILOT_AI_PROFILE` 是主要開關；舊的 `AI_DEFAULT_MODE / AI_PROVIDER / PROMPTGUARD_*` 仍保留相容 fallback，但不建議再當成日常啟動方式。

## AI Profile 對照表

| Profile | 主 AI | PromptGuard | 用途 |
| --- | --- | --- | --- |
| `1` | `preview_full` | cloud gemma | 看完整 preview，profile astrology 仍先過 guard |
| `2` | `preview_prompt_body_only` | cloud gemma | 看 prompt body，profile astrology 仍先過 guard |
| `3` | `live + mock` | cloud gemma | 主回答走 mock，但 astrology guard 還是走 cloud Gemma |
| `4` | `live + openai` | cloud gemma | 最常用，guard 走 Gemma、主回答走 OpenAI |
| `5` | `live + gemma` | cloud gemma | guard 和主回答都走 hosted Gemma |
| `6` | `live + openai` | local gemma | guard 走本地 Gemma，主回答走 OpenAI |
| `7` | `live + gemma` | local gemma | guard 走本地 Gemma，主回答走 hosted Gemma |

補充：
- profile `1~5` 的 promptguard 都是 cloud Gemma。
- profile `6~7` 的 promptguard 都是 local Gemma，預設 base URL 為 `http://localhost:11434`。
- 主 AI 預設 model：
  - OpenAI: `gpt-4o`
  - Gemma: `gemma-4-31b-it`

## 需要哪些 API key

### 一定建議先設

- `GEMINI_API_KEY`
  - promptguard cloud 會用到
  - 主 AI 若走 hosted Gemma 也會用到
  - 若同時設了舊的 `INTERNAL_AI_COPILOT_GEMMA_API_KEY` / `INTERNAL_AI_COPILOT_PROMPTGUARD_API_KEY`，現在會優先吃 `GEMINI_API_KEY`

### 主 AI 走 OpenAI 時才需要

- `OPENAI_API_KEY`

## cmd.exe 啟動指令

以下全部都是 `cmd.exe` 單行版，可直接貼。

### 1. `AI_PROFILE=1`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=1" && go run .\cmd\api
```

### 2. `AI_PROFILE=2`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=2" && go run .\cmd\api
```

### 3. `AI_PROFILE=3`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=3" && go run .\cmd\api
```

### 4. `AI_PROFILE=4`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "OPENAI_API_KEY=sk-你的-openai-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=4" && go run .\cmd\api
```

### 5. `AI_PROFILE=5`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=5" && go run .\cmd\api
```

### 6. `AI_PROFILE=6`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "OPENAI_API_KEY=sk-你的-openai-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=6" && go run .\cmd\api
```

### 7. `AI_PROFILE=7`

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "GEMINI_API_KEY=你的-gemini-api-key" && set "INTERNAL_AI_COPILOT_AI_PROFILE=7" && go run .\cmd\api
```

## 最常用組合

- 看完整 prompt：`AI_PROFILE=1`
- 只看 prompt body：`AI_PROFILE=2`
- 主回答走 mock、但 promptguard 仍驗：`AI_PROFILE=3`
- promptguard 走 Gemma、主回答走 OpenAI：`AI_PROFILE=4`
- 全部都走 Gemma family：`AI_PROFILE=5`

## 補充

- 如果沿用同一個 `cmd.exe` 視窗，`set` 過的值會保留。切 profile 時建議整段重貼。
- profile `6` / `7` 的 local promptguard 會直接打 `http://localhost:11434`；本地 endpoint 若不是這個位址，才需要回退使用舊版 env 做兼容調整。
- 若 `INTERNAL_AI_COPILOT_AI_PROFILE` 缺失或非法，runtime 仍會回退讀舊版 env，相容既有設定。
- Firestore emulator 預設：`localhost:8090`
