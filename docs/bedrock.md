# AWS Bedrock Provider

kubectl-ai supports AWS Bedrock models as a command line tool and also provides a Client API for programmatic use. 

## Usage

```bash
# Use default model (Claude Sonnet 4)
kubectl-ai --provider bedrock "explain this deployment"

# Specify model explicitly
kubectl-ai --provider bedrock --model us.anthropic.claude-3-7-sonnet-20250219-v1:0 "help me debug this pod"
```
- For more details on usage as a command line tool (i.e. kubectl-ai), see [Home Page Readme]( ../README.md)
- For more details on usage for development, as a docker build also, see [Home Page Readme]( ../README.md)
- For more detailed usage as a Client API to call from Go programs, see [Gollm Page Readme](../gollm/README.md)

## Supported Models

See [AWS Bedrock documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html) for current model availability and regional support.

The tool supports all models where the output modality is TEXT. [Here is a list of models available in Bedrock. ](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html) 

However only a handful are useful for the current usecase, since they should be TEXT models, also if you intend to use multi-turn chat (which is built on streaming) then models need to support streaming; further if you want to use tools to complete your response then models needs to support tools. [Here is a list of models and supported features](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html).

In this release, Anthropic Claude models are not supported, including Claude Sonnet 4 and Claude 3.7. Open a ticket if you need that and I will enable that.

Currently supported:

- Google Gemma 3: `google.gemma-3-4b-it`
- Google Gemma 3: `google.gemma-3-27b-it`
- Qwen3 Coder 480B: `qwen.qwen3-coder-480b-a35b-v1:0`
- Qwen3 VL 235B : `qwen.qwen3-235b-a22b-2507-v1:0`
- Nvidia Nemotron Nano : `nvidia.nemotron-nano-3-30b`
- Minimax : `nvidia.nemotron-nano-3-30b`
- Mistral AI : `mistral.voxtral-mini-3b-2507`
- Deepseek V3 : `deepseek.v3-v1:0`
- Amazon Titan Text Lite : `amazon.titan-text-lite-v1`

Many of these models do not support tools.

## Setup

### AWS Credentials

Configure AWS credentials using standard AWS SDK methods:

```bash
# Using Environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"
```

### Model Configuration

```bash
# Optional: Set default model
export BEDROCK_MODEL="us.anthropic.claude-3-7-sonnet-20250219-v1:0"
```


## Authentication

kubectl-ai uses the standard AWS SDK credential provider chain:

1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
2. AWS credentials file (~/.aws/credentials)
3. AWS config file (~/.aws/config)
4. IAM roles for EC2 instances
5. IAM roles for ECS tasks
6. IAM roles for Lambda functions

For more details, see [AWS SDK Go Configuration](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/).

## Region Configuration

Bedrock is available in specific AWS regions. Set your region using:

```bash
export AWS_REGION="us-east-1"  # Primary Bedrock region
```

Alternatively, configure region in `~/.aws/config`:

```ini
[default]
region = us-east-1
```
