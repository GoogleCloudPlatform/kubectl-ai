# kubectl-ai

[![Go Report Card](https://goreportcard.com/badge/github.com/GoogleCloudPlatform/kubectl-ai)](https://goreportcard.com/report/github.com/GoogleCloudPlatform/kubectl-ai)
![GitHub License](https://img.shields.io/github/license/GoogleCloudPlatform/kubectl-ai)
[![GitHub stars](https://img.shields.io/github/stars/GoogleCloudPlatform/kubectl-ai.svg)](https://github.com/GoogleCloudPlatform/kubectl-ai/stargazers)

`kubectl-ai` acts as an intelligent interface, translating user intent into
precise Kubernetes operations, making Kubernetes management more accessible and
efficient.

![kubectl-ai demo GIF using: kubectl-ai "how's nginx app doing in my cluster"](./.github/kubectl-ai.gif)

## Quick Start

First, ensure that kubectl is installed and configured.

### Installation

#### Quick Install (Linux & MacOS only)

```shell
curl -sSL https://raw.githubusercontent.com/GoogleCloudPlatform/kubectl-ai/main/install.sh | bash
```

<details>

<summary>Other Installation Methods</summary>

#### Manual Installation (Linux, MacOS and Windows)

1. Download the latest release from the [releases page](https://github.com/GoogleCloudPlatform/kubectl-ai/releases/latest) for your target machine.

2. Untar the release, make the binary executable and move it to a directory in your $PATH (as shown below).

```shell
tar -zxvf kubectl-ai_Darwin_arm64.tar.gz
chmod a+x kubectl-ai
sudo mv kubectl-ai /usr/local/bin/
```

#### Install with Krew (Linux/macOS/Windows)
First of all, you need to have krew insatlled, refer to [krew document](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) for more details
Then you can install with krew
```shell
kubectl krew install ai
```
Now you can invoke `kubectl-ai` as a kubectl plugin like this: `kubectl ai`.
</details>

### Usage

`kubectl-ai` supports AI models from `gemini`, `vertexai`, `azopenai`, `openai`, `grok` and local LLM providers such as `ollama` and `llama.cpp`.

#### Using Gemini (Default)

Set your Gemini API key as an environment variable. If you don't have a key, get one from [Google AI Studio](https://aistudio.google.com).

```bash
export GEMINI_API_KEY=your_api_key_here
kubectl-ai

# Use different gemini model
kubectl-ai --model gemini-2.5-pro-exp-03-25

# Use 2.5 flash (faster) model
kubectl-ai --quiet --model gemini-2.5-flash-preview-04-17 "check logs for nginx app in hello namespace"
```

<details>

<summary>Use other AI models</summary>

#### Using AI models running locally (ollama or llama.cpp)

You can use `kubectl-ai` with AI models running locally. `kubectl-ai` supports [ollama](https://ollama.com/) and [llama.cpp](https://github.com/ggml-org/llama.cpp) to use the AI models running locally.

Additionally, the [`modelserving`](modelserving/) directory provides tools and instructions for deploying your own `llama.cpp`-based LLM serving endpoints locally or on a Kubernetes cluster. This allows you to host models like Gemma directly in your environment.

An example of using Google's `gemma3` model with `ollama`:

```shell
# assuming ollama is already running and you have pulled one of the gemma models
# ollama pull gemma3:12b-it-qat

# if your ollama server is at remote, use OLLAMA_HOST variable to specify the host
# export OLLAMA_HOST=http://192.168.1.3:11434/

# enable-tool-use-shim because models require special prompting to enable tool calling
kubectl-ai --llm-provider ollama --model gemma3:12b-it-qat --enable-tool-use-shim

# you can use `models` command to discover the locally available models
>> models
```

#### Using Grok

You can use X.AI's Grok model by setting your X.AI API key:

```bash
export GROK_API_KEY=your_xai_api_key_here
kubectl-ai --llm-provider=grok --model=grok-3-beta
```

#### Using Azure OpenAI

You can also use Azure OpenAI deployment by setting your OpenAI API key and specifying the provider:

```bash
export AZURE_OPENAI_API_KEY=your_azure_openai_api_key_here
export AZURE_OPENAI_ENDPOINT=https://your_azure_openai_endpoint_here
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here
# or
az login
kubectl-ai --llm-provider=openai://your_azure_openai_endpoint_here --model=your_azure_openai_deployment_name_here
```

#### Using OpenAI

You can also use OpenAI models by setting your OpenAI API key and specifying the provider:

```bash
export OPENAI_API_KEY=your_openai_api_key_here
kubectl-ai --llm-provider=openai --model=gpt-4.1
```

#### Using OpenAI Compatible API
For example, you can use aliyun qwen-xxx models as follows
```bash
export OPENAI_API_KEY=your_openai_api_key_here
export OPENAI_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1
kubectl-ai --llm-provider=openai --model=qwen-plus
```
</details>

Run interactively:

```shell
kubectl-ai
```

The interactive mode allows you to have a chat with `kubectl-ai`, asking multiple questions in sequence while maintaining context from previous interactions. Simply type your queries and press Enter to receive responses. To exit the interactive shell, type `exit` or press Ctrl+C.

Or, run with a task as input:

```shell
kubectl-ai --quiet "fetch logs for nginx app in hello namespace"
```

Combine it with other unix commands:

```shell
kubectl-ai < query.txt
# OR
echo "list pods in the default namespace" | kubectl-ai
```

You can even combine a positional argument with stdin input. The positional argument will be used as a prefix to the stdin content:

```shell
cat error.log | kubectl-ai "explain the error"
```

## Tools

`kubectl-ai` leverages LLMs to suggest and execute Kubernetes operations using a set of powerful tools. It comes with built-in tools like `kubectl`, `bash`, and `trivy`.

You can also extend its capabilities by defining your own custom tools. By default, `kubectl-ai` looks for your tool configurations in `~/.config/kubectl-ai/tools.yaml`.

To specify a different configuration file, use:

```shell
kubectl-ai --custom-tools-config=YOUR_CONFIG
```

Define your custom tools using the following schema. You can include multiple tools in a single configuration file:

```yaml
- name: tool_name
  description: "A clear description that helps the LLM understand when to use this tool."
  command: "your_command" # For example: 'gcloud' or 'gcloud container clusters'
  command_desc: "Detailed information for the LLM, including command syntax and usage examples."
```

## Extras

You can use the following special keywords for specific actions:

* `model`: Display the currently selected model.
* `models`: List all available models.
* `tools`: List all available tools.
* `version`: Display the `kubectl-ai` version.
* `reset`: Clear the conversational context.
* `clear`: Clear the terminal screen.
* `exit` or `quit`: Terminate the interactive shell (Ctrl+C also works).

### Invoking as kubectl plugin

You can also run `kubectl ai`. `kubectl` finds any executable file in your `PATH` whose name begins with `kubectl-` as a [plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/).

## MCP server

You can also use `kubectl-ai` as a MCP server that exposes `kubectl` as one of the tools to interact with locally configured k8s environment. See [mcp docs](./docs/mcp.md) for more details.

## k8s-bench

kubectl-ai project includes [k8s-bench](./k8s-bench/README.md) - a benchmark to evaluate performance of different LLM models on kubernetes related tasks. Here is a summary from our last run:

| Model | Success | Fail |
|-------|---------|------|
| gemini-2.5-flash-preview-04-17 | 10 | 0 |
| gemini-2.5-pro-preview-03-25 | 10 | 0 |
| gemma-3-27b-it | 8 | 2 |
| **Total** | 28 | 2 |

See [full report](./k8s-bench.md) for more details.

## Start Contributing

We welcome contributions to `kubectl-ai` from the community. Take a look at our
[contribution guide](contributing.md) to get started.

---

*Note: This is not an officially supported Google product. This project is not
eligible for the [Google Open Source Software Vulnerability Rewards
Program](https://bughunters.google.com/open-source-security).*
