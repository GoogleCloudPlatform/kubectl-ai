# Bedrock Provider

kubectl-ai supports a sub-set of models available in Bedrock when used as a command line tool, within a docker container, or when a Go program uses `bedrockClient` API. [Bedrock](https://aws.amazon.com/bedrock/) is the Generative AI Platform for Amazon Web Services.

## Usage

```bash
# Specify provider and model explicitly
kubectl-ai --provider bedrock --model google.gemma-3-4b-it "help me debug this pod"
```
For more details on usage as a command line tool (i.e. kubectl-ai), see [Home Page Readme]( ../README.md)

```bash
docker run --rm -it -v ~/.kube:/root/.kube -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_REGION kubectl-ai:latest --llm-provider=bedrock --model google.gemma-3-4b-it
```
For more details on running kubectl-ai in container, as a docker build also, see [Home Page Readme]( ../README.md)

For more details on programmatic usage by using `bedrockClient`  , see [Gollm Page Readme](../gollm/README.md)

## Supported Models

[See this](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html)  for a list of models available on Bedrock. However a small subset of these models are supported by `kubectl-ai`.

The models which can be used by kubectl-ai should support TEXT `OutputModality` because of nature of the tool. For using kubectl-ai in command line mode or from within the docker container the model should support streaming too; further if you want to use tools to complete your response then models needs to support tools in streamig mode. The limit the models which can used with this tool.

Further, in this release, only models which support `ON_DEMAND` throughput are supported. Consequently Anthropic Claude models are not supported, including Claude Sonnet 4 and Claude 3.7. If this is required for your usecase, please raise a ticket with specific needs and we will support it. The list that follows gives a more accuratre list of models which should support all use cases. (Its not exhaustive, also it might be different for your region; the list was built for ap-south-1 (Mumbai) region.

#### Some models currently supported:

- Amazon Titan Text Lite : `amazon.titan-text-lite-v1`
- Google Gemma 3: `google.gemma-3-4b-it`
- Qwen3 Coder 480B: `qwen.qwen3-coder-480b-a35b-v1:0`
- Nvidia Nemotron Nano : `nvidia.nemotron-nano-3-30b`
- Mistral AI Voxtral: `mistral.voxtral-mini-3b-2507`
- Deepseek V3 : `deepseek.v3-v1:0`

If you use the `bedrockClient` API, or use Single turn implementation then the subset of models widens as this doesn't need models which support streaming. [This list](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html) should allow you to shortlist which models can be used for your usecase. 


## Setup

### Authentication

To use the Bedrock provider, we need a AWS IAM User, configured such that the user has `AmazonBedrockFullAccess` policy attached to it. We also need to create an `Access Key` for this user, and it's corresponding `Secret Access Key`. 

<details>
  <summary>Creating and Setting up AWS User</summary>

  If you don't have an IAM User, use this sections to create an IAM User, configure it with `AmazonBedrockFullAccess` policy ; if you have a IAM User with correctly configured policy use create an `Acess Key`and it's corresponding `Secret Access Key` in section below. 

  - Sign into AWS Console (with root user account)
  - Goto -> IAM (Identity and Access Management) Service
  
  #### Creating an IAM User, attaching AmazonBedrockFullAccess policy
 
  - select the User link on side panel, press "Create User" button
  - provide "User name" in text field, press "Next" button
  - on "Set Permission" page, select "Attach policies directly" choice
  - search and attach policy `AmazonBedrockFullAccess`, press "Next" button
  - on "Review and create" page, press "Create User" button


  #### For existing IAM User, create Access Key
  - Select the created IAM User on the right side listing page
  - In the "Summary" panel, press "Select access key" to create "Access Key 1"
  - In "Access key best practises & alternatives" page, select "Third-party service", also select "Confirmation", press "Next"
  - In "Description tag" value, you can leave blank, press "Create Access key"
  - Save the `Acess Key` as `AWS_ACCESS_KEY_ID`
  - Click on show to view `Secret Access Key`, save as `AWS_SECRET_ACCESS_KEY`
   
</details>

#### Configure AWS credentials in your shell as below

```bash
# Using Environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-access-key"
```

### Model Configuration

This is required if you are planning to use `bedrockClient` API programatically

```bash
# Required: For using bedrockClient API
export BEDROCK_MODEL="google.gemma-3-4b-it"
```

### Region Configuration

Bedrock is available in specific AWS regions. 
Set your region using:

```bash
export AWS_REGION="ap-south-1"  # Primary Bedrock region
```

