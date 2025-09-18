# kubectl-ai

[![Go Report Card](https://goreportcard.com/badge/github.com/GoogleCloudPlatform/kubectl-ai)](https://goreportcard.com/report/github.com/GoogleCloudPlatform/kubectl-ai)
![GitHub License](https://img.shields.io/github/license/GoogleCloudPlatform/kubectl-ai)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/GoogleCloudPlatform/kubectl-ai)
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

#### Install on NixOS

There are multiple ways to install `kubectl-ai` on NixOS. For a permantent installation add the following to your NixOS-Configuration:

```nix
  environment.systemPackages = with pkgs; [
    kubectl-ai
  ];
```

For a temporary installation, you can use the following command:

```
nix-shell -p kubectl-ai
```

</details>

### Usage

`kubectl-ai` supports AI models from `gemini`, `vertexai`, `azopenai`, `openai`, `grok`, `bedrock` and local LLM providers such as `ollama` and `llama.cpp`.

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

#### Using AWS Bedrock

You can use AWS Bedrock Claude models with your AWS credentials:

```bash
# Configure AWS credentials using AWS SSO
aws sso login --profile your-profile-name
# Or use other AWS credential methods (IAM roles, environment variables, etc.)

# Use Claude 4 Sonnet (default)
kubectl-ai --llm-provider=bedrock --model=us.anthropic.claude-sonnet-4-20250514-v1:0

# Use Claude 3.7 Sonnet
kubectl-ai --llm-provider=bedrock --model=us.anthropic.claude-3-7-sonnet-20250219-v1:0

# Override model via environment variable
export BEDROCK_MODEL=us.anthropic.claude-sonnet-4-20250514-v1:0
kubectl-ai --llm-provider=bedrock
```

AWS Bedrock uses the standard AWS SDK credential chain, supporting:
- AWS SSO profiles
- IAM roles (for EC2/ECS/Lambda)
- Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
- AWS CLI configuration files

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

We also support persistence between runs with an opt-in. This lets you save a session to the local filesystem, and resume it to maintain previous context. It even works between different interfaces!

```shell
kubectl-ai --new-session # start a new session
kubectl-ai --list-sessions # list all saved sessions
kubectl-ai --resume-session 20250807-510872 # resume session 20250807-510872
kubectl-ai --delete-session 20250807-510872 # delete session 20250807-510872
```

## Configuration

You can also configure `kubectl-ai` using a YAML configuration file at `~/.config/kubectl-ai/config.yaml`:

```shell
mkdir -p ~/.config/kubectl-ai/
cat <<EOF > ~/.config/kubectl-ai/config.yaml
model: gemini-2.5-flash-preview-04-17
llmProvider: gemini
toolConfigPaths: ~/.config/kubectl-ai/tools.yaml
EOF
```

Verify your configuration:

```shell
kubectl-ai --quiet model
```

<details>

<summary>More configuration Options</summary>

Here's a complete configuration file with all available options and their default values:

```yaml
# LLM provider configuration
llmProvider: "gemini"               # Default LLM provider
model: "gemini-2.5-pro-preview-06-05" # Default model
skipVerifySSL: false              # Skip SSL verification for LLM API calls

# Tool and permission settings
toolConfigPaths: ["~/.config/kubectl-ai/tools.yaml"]  # Custom tools configuration paths
approvalPolicy: auto-approve-read # Approval policy: auto-approve-read, paranoid, or yolo
enableToolUseShim: false        # Enable tool use shim for certain models

# MCP configuration
mcpServer: false                  # Run in MCP server mode
mcpClient: false                  # Enable MCP client mode
externalTools: false             # Discover external MCP tools (requires mcp-server)

# Runtime settings
maxIterations: 20                 # Maximum iterations for the agent
quiet: false                       # Run in non-interactive mode
removeWorkdir: false             # Remove temporary working directory after execution

# Kubernetes configuration
kubeconfig: "~/.kube/config"      # Path to kubeconfig file

# UI configuration
uiType: "terminal"                # UI mode: "terminal" or "web"
uiListenAddress: "localhost:8888" # Address for HTML UI server

# Prompt configuration
promptTemplateFilePath: ""      # Custom prompt template file
extraPromptPaths: []            # Additional prompt template paths

# Debug and trace settings
tracePath: "/tmp/kubectl-ai-trace.txt" # Path to trace file
```

</details>

The `approvalPolicy` setting (or the `--approval-policy` flag) controls how `kubectl-ai` requests permission before executing generated commands:

* `auto-approve-read` (default) skips prompts for commands the agent identifies as read-only.
* `paranoid` always asks for explicit approval before running any command.
* `yolo` never asks for approval—use with caution.

All these settings can be configured through either:

1. Command line flags (e.g., `--model=gemini-2.5-pro`)
2. Configuration file (`~/.config/kubectl-ai/config.yaml`)
3. Environment variables (e.g., `GEMINI_API_KEY`)

Command line flags take precedence over configuration file settings.

## Tools

`kubectl-ai` leverages LLMs to suggest and execute Kubernetes operations using a set of powerful tools. It comes with built-in tools like `kubectl` and `bash`.

You can also extend its capabilities by defining your own custom tools. By default, `kubectl-ai` looks for your tool configurations in `~/.config/kubectl-ai/tools.yaml`.

To specify tools configuration files or directories containing tools configuration files, use:

```sh
./kubectl-ai --custom-tools-config=<path-to-tools-directory> "your prompt here"
```

For further details on how to configure your own tools, [go here](docs/tools.md).

## Docker Quick Start 
This project provides a Docker image that gives you a standalone environment for running kubectl-ai, including against a GKE cluster.

### Running the container against GKE

#### Step 1: Build the Image

Clone the repository and build the image with the following command 

```bash
git clone https://github.com/GoogleCloudPlatform/kubectl-ai.git
cd kubectl-ai
docker build -t kubectl-ai:latest -f images/kubectl-ai/Dockerfile .
```

#### Step 2: Connect to Your GKE Cluster
Set up application default credentials and connect to your GKE cluster.
```bash
gcloud auth application-default login # If in a gcloud shell this is not necessary
gcloud container clusters get-credentials <cluster-name> --zone <zone>
```

#### Step 3: Run the kubectl-ai container
Below is a sample command that can be used to launch the container with a locally hosted web-ui. Be sure to replace the placeholder values with your specific Google Cloud project ID and location. Note you 
do not need to mount the gcloud config directory if you're on a cloudshell machine. 

```bash
docker run --rm -it -p 8080:8080 -v ~/.kube:/root/.kube -v ~/.config/gcloud:/root/.config/gcloud -e GOOGLE_CLOUD_LOCATION=us-central1 -e GOOGLE_CLOUD_PROJECT=my-gcp-project kubectl-ai:latest --llm-provider vertexai --ui-listen-address 0.0.0.0:8080 --ui-type web
```

For more info about running from the container image see [CONTAINER.md](CONTAINER.md)

## MCP Client Mode

> **Note:** MCP Client Mode is available in `kubectl-ai` version v0.0.12 and onwards.

`kubectl-ai` can connect to external [MCP](https://modelcontextprotocol.io/examples) Servers to access additional tools in addition to built-in tools.

### Quick Start

Enable MCP client mode:

```bash
kubectl-ai --mcp-client
```

### Configuration

Create or edit `~/.config/kubectl-ai/mcp.yaml` to customize MCP servers:

```yaml
servers:
  # Local MCP server (stdio-based)
  # sequential-thinking: Advanced reasoning and step-by-step analysis
  - name: sequential-thinking
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-sequential-thinking"
  
  # Remote MCP server (HTTP-based)
  - name: cloudflare-documentation
    url: https://docs.mcp.cloudflare.com/mcp
    
  # Optional: Remote MCP server with authentication
  - name: custom-api
    url: https://api.example.com/mcp
    auth:
      type: "bearer"
      token: "${MCP_TOKEN}"
```

The system automatically:

- Converts parameter names (snake_case → camelCase)
- Handles type conversion (strings → numbers/booleans when appropriate)
- Provides fallback behavior for unknown servers

No additional setup required - just use the `--mcp-client` flag and the AI will have access to all configured MCP tools.

📖 **For detailed configuration options, troubleshooting, and advanced features for MCP Client mode, see the [MCP Client Documentation](pkg/mcp/README.md).**

📖 **For multi-server orchestration and security automation examples, see the [MCP Client Integration Guide](docs/mcp-client.md).**

## Extras

You can use the following special keywords for specific actions:

- `model`: Display the currently selected model.
- `models`: List all available models.
- `tools`: List all available tools.
- `version`: Display the `kubectl-ai` version.
- `reset`: Clear the conversational context.
- `clear`: Clear the terminal screen.
- `exit` or `quit`: Terminate the interactive shell (Ctrl+C also works).

### Invoking as kubectl plugin

You can also run `kubectl ai`. `kubectl` finds any executable file in your `PATH` whose name begins with `kubectl-` as a [plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/).

## MCP Server Mode

`kubectl-ai` can act as an MCP server that exposes kubectl tools to other MCP clients (like Claude, Cursor, or VS Code). The server can run in two modes:

### Basic MCP Server (Built-in tools only)

Expose only kubectl-ai's native Kubernetes tools:

```bash
kubectl-ai --mcp-server
```

### Enhanced MCP Server (With external tool discovery)

Additionally discover and expose tools from other MCP servers as a unified interface:

```bash
kubectl-ai --mcp-server --external-tools
```

This creates a powerful **tool aggregation hub** where kubectl-ai acts as both:

- **MCP Server**: Exposing kubectl tools to clients
- **MCP Client**: Consuming tools from other MCP servers

The enhanced mode provides AI clients with access to both Kubernetes operations and general-purpose tools (filesystem, web search, databases, etc.) through a single MCP endpoint.

📖 **For detailed configuration, examples, and troubleshooting, see the [MCP Server Documentation](./docs/mcp-server.md).**

## k8s-bench

kubectl-ai project includes [k8s-bench](./k8s-bench/README.md) - a benchmark to evaluate performance of different LLM models on kubernetes related tasks. 

### Latest Benchmark Results (August 2025)

Comprehensive evaluation on identical 10-task Kubernetes benchmark with proper CNI environment:

| Model | Success | Fail | Success Rate |
|-------|---------|------|--------------|
| gemini-2.5-flash-preview-04-17 | 10 | 0 | 100% |
| gemini-2.5-pro-preview-03-25 | 10 | 0 | 100% |
| AWS Bedrock Claude 3.7 Sonnet | 10 | 0 | 100% |
| AWS Bedrock Claude Sonnet 4 | 10 | 0 | 100% |
| gemma-3-27b-it | 8 | 2 | 80% |

**Test Environment**: Kind cluster v1.27.3 with Calico CNI (full NetworkPolicy support)  
**Tasks**: create-pod, create-pod-mount-configmaps, create-pod-resources-limits, create-network-policy, fix-crashloop, fix-image-pull, fix-service-routing, list-images-for-pods, scale-deployment, scale-down-deployment

See [full report](./k8s-bench.md) for more details.

## Start Contributing

We welcome contributions to `kubectl-ai` from the community. Take a look at our
[contribution guide](contributing.md) to get started.

---

*Note: This is not an officially supported Google product. This project is not
eligible for the [Google Open Source Software Vulnerability Rewards
Program](https://bughunters.google.com/open-source-security).*
