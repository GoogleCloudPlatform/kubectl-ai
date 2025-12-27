# AWS Bedrock Provider

kubectl-ai supports AWS Bedrock models as a command line tool and also provides a Client API for programmatic use. 

For more details on usage as a command line tool see [Home Page] (../README.md) For more detailed usage as a Client API to call from Go programs, see [Gollm Page](../gollm/README.md)

The tool supports all models where the output modality is TEXT. [Here is a list of models available in Bedrock. ](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html) However only a handful are useful for the current usecase, since they should be TEXT models, also if you intend to use multi-turn chat (which is built on streaming) then models need to support streaming; further if you want to use tools to complete your response then models needs to support tools. [Here is a list of models and supported features](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html).

In this release, Anthropic Claude models are not supported, including Claude Sonnet 4 and Claude 3.7. Open a ticket if you need that and I will enable that.

## Setup

### AWS Credentials

Configure AWS credentials using standard AWS SDK methods:

```bash
# Option 1: Environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"

# Option 2: AWS Profile (recommended)
export AWS_PROFILE="your-profile-name"
export AWS_REGION="us-east-1"

# Option 3: Use IAM roles (on EC2/ECS/Lambda)
export AWS_REGION="us-east-1"
```

### Model Configuration

```bash
# Optional: Set default model
export BEDROCK_MODEL="us.anthropic.claude-3-7-sonnet-20250219-v1:0"
```

## Supported Models

See [AWS Bedrock documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html) for current model availability and regional support.

Currently supported:

- Claude Sonnet 4: `us.anthropic.claude-sonnet-4-20250514-v1:0` (default)
- Claude 3.7 Sonnet: `us.anthropic.claude-3-7-sonnet-20250219-v1:0`

## Usage

```bash
# Use default model (Claude Sonnet 4)
kubectl-ai --provider bedrock "explain this deployment"

# Specify model explicitly
kubectl-ai --provider bedrock --model us.anthropic.claude-3-7-sonnet-20250219-v1:0 "help me debug this pod"
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
