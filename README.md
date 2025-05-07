# kubectl-ai

kubectl-ai is an AI powered kubernetes agent that runs in your terminal.

![kubectl-ai demo GIF using: kubectl-ai "how's nginx app doing in my cluster"](./.github/kubectl-ai.gif)

## Quick Start

First, ensure that kubectl is installed and configured.

### Installation

1. Download the latest release from the [releases page](https://github.com/GoogleCloudPlatform/kubectl-ai/releases/latest) for your target machine.

2. Untar the release, make the binary executable and move it to a directory in your $PATH (as shown below).

```shell
tar -zxvf kubectl-ai_Darwin_arm64.tar.gz
chmod a+x kubectl-ai
sudo mv kubectl-ai /usr/local/bin/
```

### Usage

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

#### Using AI models running locally (ollama or llamacpp)

You can use `kubectl-ai` with AI models running locally. `kubectl-ai` supports [ollama](https://ollama.com/) and [llama.cpp](https://github.com/ggml-org/llama.cpp) to use the AI models running locally.

An example of using Google's `gemma3` model with `ollama`:

```shell
# assuming ollama is already running and you have pulled one of the gemma models
# ollama pull gemma3:12b-it-qat

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
For example, you can use aliyun qwen-xxx module as follows
```bash
export OPENAI_API_KEY=your_openai_api_key_here
export OPENAI_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1
kubectl-ai --llm-provider=openai --model=qwen-plus
```

* Note: `kubectl-ai` supports AI models from `gemini`, `vertexai`, `azopenai`, `openai`, `grok` and local LLM providers such as `ollama` and `llamacpp`.

Run interactively:

```shell
kubectl-ai
```

The interactive mode allows you to have a chat with `kubectl-ai`, asking multiple questions in sequence while maintaining context from previous interactions. Simply type your queries and press Enter to receive responses. To exit the interactive shell, type `exit` or press Ctrl+C.

Or, run with a task as input:

```shell
kubectl-ai -quiet "fetch logs for nginx app in hello namespace"
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

## Extras

You can use the following special keywords for specific actions:

* `model`: Display the currently selected model.
* `models`: List all available models.
* `version`: Display the `kubectl-ai` version.
* `reset`: Clear the conversational context.
* `clear`: Clear the terminal screen.
* `exit` or `quit`: Terminate the interactive shell (Ctrl+C also works).

### Invoking as kubectl plugin

Use it via the `kubectl` plug interface like this: `kubectl ai`.  kubectl will find `kubectl-ai` as long as it's in your PATH.  For more information about plugins please see: https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/


### Examples

```bash
# Get information about pods in the default namespace
kubectl-ai -quiet "show me all pods in the default namespace"

# Create a new deployment
kubectl-ai -quiet "create a deployment named nginx with 3 replicas using the nginx:latest image"

# Troubleshoot issues
kubectl-ai -quiet "double the capacity for the nginx app"

# Using Azure OpenAI instead of Gemini
kubectl-ai --llm-provider=azopenai --model=your_azure_openai_deployment_name_here -quiet "scale the nginx deployment to 5 replicas"

# Using OpenAI instead of Gemini
kubectl-ai --llm-provider=openai --model=gpt-4.1 -quiet "scale the nginx deployment to 5 replicas"
```

The `kubectl-ai` will process your query, execute the appropriate kubectl commands, and provide you with the results and explanations.

## k8s-bench

kubectl-ai project includes [k8s-bench](./k8s-bench/README.md) - a benchmark to evaluate performance of different LLM models on kubernetes related tasks. Here is a summary from our last run:

| Model | Success | Fail |
|-------|---------|------|
| gemini-2.5-flash-preview-04-17 | 10 | 0 |
| gemini-2.5-pro-preview-03-25 | 10 | 0 |
| gemma-3-27b-it | 8 | 2 |
| **Total** | 28 | 2 |

See [full report](./k8s-bench.md) for more details.

---

*Note: This is not an officially supported Google product. This project is not
eligible for the [Google Open Source Software Vulnerability Rewards
Program](https://bughunters.google.com/open-source-security).*
