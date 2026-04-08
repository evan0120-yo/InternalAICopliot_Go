# Internal AI Copilot Go Backend

## 啟動前先知道

- Go 專案路徑：`D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go`
- 目前 AI 執行邏輯：

```text
analyze
│
├─ preview_full
├─ preview_prompt_body_only
└─ live
   ├─ mock
   └─ provider
      ├─ openai
      └─ gemma
```

- 不設 env 時，預設會走：

```text
AI_DEFAULT_MODE = live
AI_MOCK_MODE    = false
AI_PROVIDER     = openai
```

- 下方啟動指令預設是給 `cmd.exe` 用的。
- 如果沿用同一個 `cmd.exe` 視窗，`set` 過的環境變數會保留。切模式時，建議整段重貼，不要只改一行。

## 常用環境變數

- `INTERNAL_AI_COPILOT_AI_DEFAULT_MODE`
  - `preview_full`
  - `preview_prompt_body_only`
  - `live`
- `INTERNAL_AI_COPILOT_AI_MOCK_MODE`
  - `true`
  - `false`
- `INTERNAL_AI_COPILOT_AI_PROVIDER`
  - `openai`
  - `gemma`
- `OPENAI_API_KEY`
- `OPENAI_BASE_URL`
- `INTERNAL_AI_COPILOT_AI_MODEL`
- `INTERNAL_AI_COPILOT_GEMMA_API_KEY`
- `INTERNAL_AI_COPILOT_GEMMA_BASE_URL`
- `INTERNAL_AI_COPILOT_GEMMA_MODEL`

## cmd.exe 啟動指令

以下指令可直接貼到 Windows 命令提示字元 `cmd.exe`。
全部都是單行版本。

### 1. 預設啟動

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=" && set "INTERNAL_AI_COPILOT_AI_MODEL=" && set "INTERNAL_AI_COPILOT_GEMMA_MODEL=" && set "OPENAI_BASE_URL=" && set "INTERNAL_AI_COPILOT_GEMMA_BASE_URL=" && go run .\cmd\api
```

### 2. preview_full

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=preview_full" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && go run .\cmd\api
```

### 3. preview_prompt_body_only

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=preview_prompt_body_only" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && go run .\cmd\api
```

### 4. live + mock

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=true" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && go run .\cmd\api
```

### 5. live + openai，使用預設 model

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && set "INTERNAL_AI_COPILOT_AI_MODEL=" && set "OPENAI_BASE_URL=" && set "OPENAI_API_KEY=sk-你的-openai-key" && go run .\cmd\api
```

### 6. live + openai，指定 model

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && set "INTERNAL_AI_COPILOT_AI_MODEL=gpt-4.1-mini" && set "OPENAI_BASE_URL=" && set "OPENAI_API_KEY=sk-你的-openai-key" && go run .\cmd\api
```

### 7. live + gemma，使用預設 model

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=gemma" && set "INTERNAL_AI_COPILOT_GEMMA_MODEL=" && set "INTERNAL_AI_COPILOT_GEMMA_BASE_URL=" && set "INTERNAL_AI_COPILOT_GEMMA_API_KEY=你的-gemma-api-key" && go run .\cmd\api
```

### 8. live + gemma，指定 model

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=gemma" && set "INTERNAL_AI_COPILOT_GEMMA_MODEL=gemma-4-31b-it" && set "INTERNAL_AI_COPILOT_GEMMA_BASE_URL=" && set "INTERNAL_AI_COPILOT_GEMMA_API_KEY=你的-gemma-api-key" && go run .\cmd\api
```

### 9. live + openai，自訂 base URL

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=openai" && set "INTERNAL_AI_COPILOT_AI_MODEL=gpt-4o" && set "OPENAI_BASE_URL=https://你的-openai-compatible-endpoint/v1" && set "OPENAI_API_KEY=sk-你的-openai-key" && go run .\cmd\api
```

### 10. live + gemma，自訂 base URL

```bat
cd /d "D:\WorkSpace\ProjectAI\InternalAICopliot\Backend\Go" && set "INTERNAL_AI_COPILOT_AI_DEFAULT_MODE=live" && set "INTERNAL_AI_COPILOT_AI_MOCK_MODE=false" && set "INTERNAL_AI_COPILOT_AI_PROVIDER=gemma" && set "INTERNAL_AI_COPILOT_GEMMA_MODEL=gemma-4-31b-it" && set "INTERNAL_AI_COPILOT_GEMMA_BASE_URL=https://generativelanguage.googleapis.com/v1beta" && set "INTERNAL_AI_COPILOT_GEMMA_API_KEY=你的-gemma-api-key" && go run .\cmd\api
```

## 補充

- Gemma API Key 讀取順序：

```text
INTERNAL_AI_COPILOT_GEMMA_API_KEY
-> REWARDBRIDGE_GEMMA_API_KEY
-> GEMINI_API_KEY
-> GOOGLE_API_KEY
```

- OpenAI model 預設值：`gpt-4o`
- Gemma model 預設值：`gemma-4-31b-it`
- Firestore emulator 預設：`localhost:8090`
