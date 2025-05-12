# kubectl-ai

English | [繁體中文](README.zh-TW.md) | [简体中文](README.zh-CN.md)

`kubectl-ai` 是一个智能接口，可以将用户意图转化为精确的 Kubernetes 操作，让 Kubernetes 管理变得更加简单高效。

![kubectl-ai demo GIF using: kubectl-ai "how's nginx app doing in my cluster"](./.github/kubectl-ai.gif)

## 快速开始

首先，请确保已安装并配置好 kubectl。

### 安装方式

#### 快速安装（仅限 Linux & MacOS）

```shell
curl -sSL https://raw.githubusercontent.com/GoogleCloudPlatform/kubectl-ai/main/install.sh | bash
```

#### 手动安装（Linux、MacOS 和 Windows）

1. 从 [releases page](https://github.com/GoogleCloudPlatform/kubectl-ai/releases/latest) 下载适合你设备的最新版本。

2. 解压缩文件，将二进制文件设为可执行，并移动到 $PATH 目录下（如下所示）。

```shell
tar -zxvf kubectl-ai_Darwin_arm64.tar.gz
chmod a+x kubectl-ai
sudo mv kubectl-ai /usr/local/bin/
```

#### 使用 Krew 安装（Linux/macOS/Windows）

首先需要安装 krew，参考 [krew 文档](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) 获取详细说明。
然后可以使用 krew 安装：

```shell
kubectl krew install ai
```

现在你可以像这样以 kubectl 插件方式调用 `kubectl-ai`：`kubectl ai`。

### 使用方法

#### 使用 Gemini（默认）

将你的 Gemini API 密钥设置为环境变量。如果还没有密钥，请前往 [Google AI Studio](https://aistudio.google.com) 获取。

```bash
export GEMINI_API_KEY=your_api_key_here
kubectl-ai

# 使用不同 gemini 模型
kubectl-ai --model gemini-2.5-pro-exp-03-25

# 使用 2.5 flash（更快）模型
kubectl-ai --quiet --model gemini-2.5-flash-preview-04-17 "check logs for nginx app in hello namespace"
```

#### 使用本地 AI 模型（ollama 或 llama.cpp）

你可以配合本地运行的 AI 模型使用 `kubectl-ai`。`kubectl-ai` 支持 [ollama](https://ollama.com/) 和 [llama.cpp](https://github.com/ggml-org/llama.cpp)。

以下是使用 Google 的 `gemma3` 模型配合 `ollama` 的示例：

```shell
# 假设 ollama 已经运行并且已拉取 gemma 模型
# ollama pull gemma3:12b-it-qat

# 如果 ollama server 在远程，请用 OLLAMA_HOST 变量指定主机
# export OLLAMA_HOST=http://192.168.1.3:11434/

# enable-tool-use-shim 因为模型需要特殊提示以启用工具调用
kubectl-ai --llm-provider ollama --model gemma3:12b-it-qat --enable-tool-use-shim

# 可以用 `models` 命令查看本地可用模型
>> models
```

#### 使用 Grok

可以通过设置 X.AI API 密钥来使用 X.AI 的 Grok 模型：

```bash
export GROK_API_KEY=your_xai_api_key_here
kubectl-ai --llm-provider=grok --model=grok-3-beta
```

#### 使用 Azure OpenAI

也可以设置 OpenAI API 密钥并指定 provider 来使用 Azure OpenAI：

```bash
export AZURE_OPENAI_API_KEY=your_azure_openai_api_key_here
export AZURE_OPENAI_ENDPOINT=https://your_azure_openai_endpoint_here
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here
# 或
az login
kubectl-ai --llm-provider=openai://your_azure_openai_endpoint_here --model=your_azure_openai_deployment_name_here
```

#### 使用 OpenAI

也可以设置 OpenAI API 密钥并指定 provider 来使用 OpenAI 模型：

```bash
export OPENAI_API_KEY=your_openai_api_key_here
kubectl-ai --llm-provider=openai --model=gpt-4.1
```

#### 使用 OpenAI 兼容 API

例如，可以使用阿里云 qwen-xxx 模型如下：

```bash
export OPENAI_API_KEY=your_openai_api_key_here
export OPENAI_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1
kubectl-ai --llm-provider=openai --model=qwen-plus
```

- 注意：`kubectl-ai` 支持来自 `gemini`、`vertexai`、`azopenai`、`openai`、`grok` 以及本地 LLM provider（如 `ollama`、`llama.cpp`）的 AI 模型。

交互模式运行：

```shell
kubectl-ai
```

交互模式允许你与 `kubectl-ai` 进行多轮对话，保留上下文。直接输入问题并按 Enter 即可获得回复。退出交互 shell 输入 `exit` 或按 Ctrl+C。

或以任务作为输入运行：

```shell
kubectl-ai --quiet "fetch logs for nginx app in hello namespace"
```

可以与其他 unix 命令结合：

```shell
kubectl-ai < query.txt
# 或
echo "list pods in the default namespace" | kubectl-ai
```

还可以将位置参数与 stdin 输入结合。位置参数会作为 stdin 内容的前缀：

```shell
cat error.log | kubectl-ai "explain the error"
```

## 其他功能

你可以使用以下特殊关键字执行特定操作：

- `model`：显示当前选用的模型
- `models`：列出所有可用模型
- `version`：显示 `kubectl-ai` 版本
- `reset`：清除对话上下文
- `clear`：清除终端屏幕
- `exit` 或 `quit`：退出交互 shell（Ctrl+C 也可）

### 作为 kubectl 插件调用

可以通过 `kubectl` 插件接口调用：`kubectl ai`。只要 `kubectl-ai` 在 PATH 中即可被 kubectl 识别。更多插件信息请见：[kubectl-plugins](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)

### 示例

```bash
# 查询 default namespace 中的 pods
kubectl-ai --quiet "show me all pods in the default namespace"

# 创建新 deployment
kubectl-ai --quiet "create a deployment named nginx with 3 replicas using the nginx:latest image"

# 故障排查
kubectl-ai --quiet "double the capacity for the nginx app"

# 使用 Azure OpenAI 替代 Gemini
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here --quiet "scale the nginx deployment to 5 replicas"

# 使用 OpenAI 替代 Gemini
kubectl-ai --llm-provider=openai --model=gpt-4.1 --quiet "scale the nginx deployment to 5 replicas"
```

`kubectl-ai` 会处理你的查询，执行相应的 kubectl 命令，并提供结果和说明。

## MCP server

你也可以将 `kubectl-ai` 作为 MCP server，将 `kubectl` 暴露为工具以交互本地 k8s 环境。详见 [mcp docs](./docs/mcp.md)。

## k8s-bench

kubectl-ai 项目包含 [k8s-bench](./k8s-bench/README.md) —— 用于评估不同 LLM 模型在 Kubernetes 相关任务上的性能。以下为最近一次测试摘要：

| Model                          | Success | Fail |
| ------------------------------ | ------- | ---- |
| gemini-2.5-flash-preview-04-17 | 10      | 0    |
| gemini-2.5-pro-preview-03-25   | 10      | 0    |
| gemma-3-27b-it                 | 8       | 2    |
| **Total**                      | 28      | 2    |

详见 [完整报告](./k8s-bench.md)。

## 开始贡献

欢迎社区贡献 `kubectl-ai`，请参阅 [贡献指南](contributing.md) 开始参与。

---

_注意：这不是 Google 官方支持的产品。本项目不适用于 [Google Open Source Software Vulnerability Rewards Program](https://bughunters.google.com/open-source-security)。_
