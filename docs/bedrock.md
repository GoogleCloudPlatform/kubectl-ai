# AWS Bedrock Provider

kubectl-ai supports AWS Bedrock models as a command line tool and also provides a Client API for programmatic use. 

## Usage

```bash
# Specify provider and model explicitly
kubectl-ai --provider bedrock --model google.gemma-3-4b-it "help me debug this pod"
```
For more details on usage as a command line tool (i.e. kubectl-ai), see [Home Page Readme]( ../README.md)

```bash
docker run --rm -it -v ~/.kube:/root/.kube -v ~/home/ubuntu/.aws:/root/.aws -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_REGION kubectl-ai:latest --llm-provider=bedrock --model google.gemma-3-4b-it
```
For more details on running kubectl-ai in container, as a docker build also, see [Home Page Readme]( ../README.md)

For more details on programmatic usage by using `bedrockClient`  , see [Gollm Page Readme](../gollm/README.md)

## Supported Models

See this for a [list of models available on Bedrock. ](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html) 

However, the models which can be used by kubectl-ai should support TEXT `OutputModality`. If you intend to use kubectl-ai as a command line tool or within the docker container, this works on multi-turn chat which is built in kubectl-ai using streaming so the model should support streaming too; further if you want to use tools to complete your response then models needs to support tools in streamig mode. Howeber if you use the `berockClient` API, or use Single turn implementation then the list of models you can use widens. [This list](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html) should allow you to shortlist which models can be used for your usecase. 

The list that follows gives a more accuratre list of models which should support all use cases. (Its not exhaustive, it might be different for your region; the list was built for ap-south-1 (Mumbai) region.

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
# Required: For using bedrockClient API
export BEDROCK_MODEL="google.gemma-3-4b-it"
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
export AWS_REGION="ap-south-1"  # Primary Bedrock region
```

Alternatively, configure region in `~/.aws/config`:

```ini
[default]
region = ap-south-1
```
