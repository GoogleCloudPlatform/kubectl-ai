# kubectl-ai

English | [繁體中文](README.zh-TW.md) | [简体中文](README.zh-CN.md)

`kubectl-ai` 是一個智慧型介面，能將使用者意圖轉換為精確的 Kubernetes 操作，讓 Kubernetes 管理變得更容易且高效。

![kubectl-ai demo GIF using: kubectl-ai "how's nginx app doing in my cluster"](./.github/kubectl-ai.gif)

## 快速開始

首先，請確保已安裝並設定好 kubectl。

### 安裝方式

#### 快速安裝（僅限 Linux & MacOS）

```shell
curl -sSL https://raw.githubusercontent.com/GoogleCloudPlatform/kubectl-ai/main/install.sh | bash
```

#### 手動安裝（Linux、MacOS 及 Windows）

1. 從 [releases page](https://github.com/GoogleCloudPlatform/kubectl-ai/releases/latest) 下載適合你機器的最新版本。

2. 解壓縮檔案，將執行檔設為可執行，並移動到 $PATH 目錄下（如下所示）。

```shell
tar -zxvf kubectl-ai_Darwin_arm64.tar.gz
chmod a+x kubectl-ai
sudo mv kubectl-ai /usr/local/bin/
```

#### 使用 Krew 安裝（Linux/macOS/Windows）

首先需安裝 krew，請參考 [krew 文件](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) 取得詳細說明。
然後即可使用 krew 安裝：

```shell
kubectl krew install ai
```

現在你可以像這樣以 kubectl plugin 方式呼叫 `kubectl-ai`：`kubectl ai`。

### 使用方式

#### 使用 Gemini（預設）

將你的 Gemini API 金鑰設為環境變數。若尚未取得金鑰，請至 [Google AI Studio](https://aistudio.google.com) 申請。

```bash
export GEMINI_API_KEY=your_api_key_here
kubectl-ai

# 使用不同 gemini 模型
kubectl-ai --model gemini-2.5-pro-exp-03-25

# 使用 2.5 flash（較快）模型
kubectl-ai --quiet --model gemini-2.5-flash-preview-04-17 "check logs for nginx app in hello namespace"
```

#### 使用本地 AI 模型（ollama 或 llama.cpp）

你可以搭配本地執行的 AI 模型使用 `kubectl-ai`。`kubectl-ai` 支援 [ollama](https://ollama.com/) 及 [llama.cpp](https://github.com/ggml-org/llama.cpp)。

以下為使用 Google 的 `gemma3` 模型搭配 `ollama` 的範例：

```shell
# 假設 ollama 已啟動且已拉取 gemma 模型
# ollama pull gemma3:12b-it-qat

# 若 ollama server 在遠端，請用 OLLAMA_HOST 變數指定主機
# export OLLAMA_HOST=http://192.168.1.3:11434/

# enable-tool-use-shim 因為模型需特殊提示以啟用工具呼叫
kubectl-ai --llm-provider ollama --model gemma3:12b-it-qat --enable-tool-use-shim

# 可用 `models` 指令查詢本地可用模型
>> models
```

#### 使用 Grok

可透過設定 X.AI API 金鑰來使用 X.AI 的 Grok 模型：

```bash
export GROK_API_KEY=your_xai_api_key_here
kubectl-ai --llm-provider=grok --model=grok-3-beta
```

#### 使用 Azure OpenAI

也可設定 OpenAI API 金鑰並指定 provider 來使用 Azure OpenAI：

```bash
export AZURE_OPENAI_API_KEY=your_azure_openai_api_key_here
export AZURE_OPENAI_ENDPOINT=https://your_azure_openai_endpoint_here
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here
# 或
az login
kubectl-ai --llm-provider=openai://your_azure_openai_endpoint_here --model=your_azure_openai_deployment_name_here
```

#### 使用 OpenAI

也可設定 OpenAI API 金鑰並指定 provider 來使用 OpenAI 模型：

```bash
export OPENAI_API_KEY=your_openai_api_key_here
kubectl-ai --llm-provider=openai --model=gpt-4.1
```

#### 使用 OpenAI 相容 API

例如，可用阿里雲 qwen-xxx 模型如下：

```bash
export OPENAI_API_KEY=your_openai_api_key_here
export OPENAI_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1
kubectl-ai --llm-provider=openai --model=qwen-plus
```

- 注意：`kubectl-ai` 支援來自 `gemini`、`vertexai`、`azopenai`、`openai`、`grok` 及本地 LLM provider（如 `ollama`、`llama.cpp`）的 AI 模型。

互動模式執行：

```shell
kubectl-ai
```

互動模式可讓你與 `kubectl-ai` 進行多輪對話，保留上下文。直接輸入問題並按 Enter 即可獲得回應。離開互動 shell 請輸入 `exit` 或按 Ctrl+C。

或以任務作為輸入執行：

```shell
kubectl-ai --quiet "fetch logs for nginx app in hello namespace"
```

可與其他 unix 指令結合：

```shell
kubectl-ai < query.txt
# 或
echo "list pods in the default namespace" | kubectl-ai
```

也可將位置參數與 stdin 輸入結合。位置參數會作為 stdin 內容的前綴：

```shell
cat error.log | kubectl-ai "explain the error"
```

## 其他功能

你可以使用以下特殊關鍵字執行特定動作：

- `model`：顯示目前選用的模型
- `models`：列出所有可用模型
- `version`：顯示 `kubectl-ai` 版本
- `reset`：清除對話上下文
- `clear`：清除終端畫面
- `exit` 或 `quit`：結束互動 shell（Ctrl+C 亦可）

### 作為 kubectl plugin 呼叫

可透過 `kubectl` plugin 介面呼叫：`kubectl ai`。只要 `kubectl-ai` 在 PATH 中即可被 kubectl 偵測。更多 plugin 資訊請見：[kubectl-plugins](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)

### 範例

```bash
# 查詢 default namespace 中的 pods
kubectl-ai --quiet "show me all pods in the default namespace"

# 建立新 deployment
kubectl-ai --quiet "create a deployment named nginx with 3 replicas using the nginx:latest image"

# 疑難排解
kubectl-ai --quiet "double the capacity for the nginx app"

# 使用 Azure OpenAI 取代 Gemini
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here --quiet "scale the nginx deployment to 5 replicas"

# 使用 OpenAI 取代 Gemini
kubectl-ai --llm-provider=openai --model=gpt-4.1 --quiet "scale the nginx deployment to 5 replicas"
```

`kubectl-ai` 會處理你的查詢，執行適當的 kubectl 指令，並提供結果與說明。

## MCP server

你也可以將 `kubectl-ai` 作為 MCP server，將 `kubectl` 暴露為工具以互動本地 k8s 環境。詳見 [mcp docs](./docs/mcp.md)。

## k8s-bench

kubectl-ai 專案包含 [k8s-bench](./k8s-bench/README.md) —— 用於評估不同 LLM 模型在 Kubernetes 任務上的效能。以下為最近一次測試摘要：

| Model                          | Success | Fail |
| ------------------------------ | ------- | ---- |
| gemini-2.5-flash-preview-04-17 | 10      | 0    |
| gemini-2.5-pro-preview-03-25   | 10      | 0    |
| gemma-3-27b-it                 | 8       | 2    |
| **Total**                      | 28      | 2    |

詳見 [完整報告](./k8s-bench.md)。

## 開始貢獻

歡迎社群貢獻 `kubectl-ai`，請參閱 [貢獻指南](contributing.md) 開始參與。

---

_注意：這不是 Google 官方支援的產品。本專案不適用於 [Google Open Source Software Vulnerability Rewards Program](https://bughunters.google.com/open-source-security)。_
