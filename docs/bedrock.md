# AWS Bedrock Integration Guide

kubectl-ai now supports AWS Bedrock, allowing you to use Amazon's foundational models for Kubernetes operations. This guide covers setup, configuration, and usage of AWS Bedrock with kubectl-ai.

## Overview

AWS Bedrock is Amazon's managed service for foundational models from leading AI companies. kubectl-ai integrates with Bedrock to provide AI-powered Kubernetes management using models like:

- **Anthropic Claude**: Advanced reasoning and code generation
- **Amazon Nova**: High-performance and cost-effective models
- **Mistral**: Multilingual and specialized models
- **Stability AI**: Image generation and processing models

## Prerequisites

1. **AWS Account**: You need an AWS account with appropriate permissions
2. **AWS CLI**: Install and configure AWS CLI (optional but recommended)
3. **Model Access**: Request access to the models you want to use in AWS Bedrock console
4. **kubectl-ai**: Version 0.0.12 or later

## Setup

### 1. AWS Credentials

Configure your AWS credentials using one of these methods:

#### Option A: AWS CLI (Recommended)
```bash
aws configure
```

#### Option B: Environment Variables
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_DEFAULT_REGION=us-west-2
```

#### Option C: AWS SSO
```bash
aws configure sso
```

#### Option D: IAM Role (for EC2/ECS/Lambda)
AWS SDK will automatically use the IAM role attached to your instance.

### 2. Required AWS Permissions

Ensure your AWS credentials have the following permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "bedrock:InvokeModel",
                "bedrock:InvokeModelWithResponseStream",
                "bedrock:GetFoundationModel",
                "bedrock:ListFoundationModels"
            ],
            "Resource": "*"
        }
    ]
}
```

### 3. Request Model Access

1. Go to the [AWS Bedrock Console](https://console.aws.amazon.com/bedrock)
2. Navigate to "Model access" in the left sidebar
3. Request access to the models you want to use
4. Wait for approval (this can take a few minutes to hours)

## Configuration

### Basic Configuration

Create or edit `~/.config/kubectl-ai/config.yaml`:

```yaml
llm-provider: bedrock
model: us.anthropic.claude-sonnet-4-20250514-v1:0
```

### Advanced Configuration

```yaml
llm-provider: bedrock
model: us.anthropic.claude-sonnet-4-20250514-v1:0
temperature: 0.7
max-tokens: 4000
top-p: 0.9
max-retries: 3
region: us-west-2
timeout: 30s
```

## Usage

### Basic Usage

```bash
# Use default Bedrock model
kubectl-ai --llm-provider bedrock

# Use specific model
kubectl-ai --llm-provider bedrock --model us.anthropic.claude-sonnet-4-20250514-v1:0

# Use with specific region
kubectl-ai --llm-provider bedrock --model anthropic.claude-v2:1 --region us-east-1
```

### Interactive Mode

```bash
kubectl-ai --llm-provider bedrock
> list all pods in kube-system namespace
> create a deployment for nginx with 3 replicas
> help me troubleshoot why my pod is pending
```

### One-shot Commands

```bash
# Quick queries
kubectl-ai --llm-provider bedrock --quiet "show me failing pods"

# Pipe input
echo "create a configmap from my .env file" | kubectl-ai --llm-provider bedrock
```

## Available Models

### By Region

The following models are available in different AWS regions:

#### US East (N. Virginia) - us-east-1
- `us.anthropic.claude-sonnet-4-20250514-v1:0`
- `us.anthropic.claude-3-7-sonnet-20250219-v1:0`
- `us.amazon.nova-pro-v1:0`
- `us.amazon.nova-lite-v1:0`
- `us.amazon.nova-micro-v1:0`
- `anthropic.claude-v2:1`
- `anthropic.claude-instant-v1`
- `amazon.nova-pro-v1:0`
- `mistral.mistral-large-2402-v1:0`

#### US West (Oregon) - us-west-2
- `us.anthropic.claude-sonnet-4-20250514-v1:0`
- `us.anthropic.claude-3-7-sonnet-20250219-v1:0`
- `us.amazon.nova-pro-v1:0`
- `us.amazon.nova-lite-v1:0`
- `us.amazon.nova-micro-v1:0`
- `anthropic.claude-v2:1`
- `amazon.nova-pro-v1:0`
- `stability.sd3-large-v1:0`

#### EU (Ireland) - eu-west-1
- `anthropic.claude-v2:1`
- `anthropic.claude-instant-v1`
- `amazon.nova-pro-v1:0`
- `amazon.nova-lite-v1:0`
- `amazon.nova-micro-v1:0`

#### EU (Frankfurt) - eu-central-1
- `anthropic.claude-v2:1`
- `anthropic.claude-instant-v1`
- `amazon.nova-pro-v1:0`
- `amazon.nova-lite-v1:0`
- `amazon.nova-micro-v1:0`

#### Asia Pacific (Singapore) - ap-southeast-1
- `anthropic.claude-v2:1`
- `anthropic.claude-instant-v1`
- `amazon.nova-pro-v1:0`
- `amazon.nova-lite-v1:0`
- `amazon.nova-micro-v1:0`

#### Asia Pacific (Tokyo) - ap-northeast-1
- `anthropic.claude-v2:1`
- `anthropic.claude-instant-v1`
- `amazon.nova-pro-v1:0`
- `amazon.nova-lite-v1:0`
- `amazon.nova-micro-v1:0`

### Model Recommendations

- **For complex reasoning**: `us.anthropic.claude-sonnet-4-20250514-v1:0`
- **For balanced performance**: `us.anthropic.claude-3-7-sonnet-20250219-v1:0`
- **For cost efficiency**: `us.amazon.nova-lite-v1:0`
- **For high throughput**: `us.amazon.nova-micro-v1:0`

## Troubleshooting

### Common Issues

#### 1. "Access Denied" Error
```bash
Error: failed to invoke Bedrock model: access denied
```

**Solution**: Ensure you have requested access to the model in the Bedrock console and have proper IAM permissions.

#### 2. "Model Not Found" Error
```bash
Error: unsupported model - only Claude and Nova models are supported
```

**Solution**: Use a supported model from the list above, or check if the model is available in your region.

#### 3. "Region Not Available" Error
```bash
Error: failed to load AWS configuration
```

**Solution**: Ensure the model is available in your specified region, or try a different region.

#### 4. Timeout Issues
```bash
Error: context deadline exceeded
```

**Solution**: Increase timeout value:
```bash
kubectl-ai --llm-provider bedrock --timeout 60s
```

### Debug Mode

Enable debug mode for detailed logging:

```bash
kubectl-ai --llm-provider bedrock --debug
```

### Verify Configuration

Check your current configuration:

```bash
kubectl-ai --llm-provider bedrock model
```

## Cost Optimization

### Token Usage Tracking

kubectl-ai provides built-in token usage tracking:

```bash
# The system automatically tracks:
# - Input tokens
# - Output tokens
# - Total cost per request
# - Provider and model information
```

### Best Practices

1. **Choose appropriate models**: Use lighter models for simple tasks
2. **Set reasonable limits**: Configure max-tokens to prevent runaway costs
3. **Use region-specific models**: Avoid cross-region data transfer costs
4. **Monitor usage**: Enable usage callbacks for cost tracking

## Advanced Features

### Custom Inference Profiles

Use AWS Bedrock Inference Profiles:

```bash
kubectl-ai --llm-provider bedrock --model "arn:aws:bedrock:us-west-2:123456789012:inference-profile/my-profile"
```

### Streaming Responses

Enable streaming for real-time responses:

```bash
kubectl-ai --llm-provider bedrock --stream
```

### Custom Timeouts

Configure custom timeouts:

```bash
kubectl-ai --llm-provider bedrock --timeout 120s
```

## Integration with Other Tools

### With MCP (Model Context Protocol)

```bash
# Use Bedrock with MCP client mode
kubectl-ai --llm-provider bedrock --mcp-client

# Use Bedrock as MCP server
kubectl-ai --llm-provider bedrock --mcp-server
```

### With Custom Tools

```yaml
# ~/.config/kubectl-ai/tools.yaml
- name: bedrock-specific-tool
  description: "Tool that works well with Bedrock models"
  command: "your-command"
  command_desc: "Detailed description for Bedrock AI"
```

## Examples

### Creating Resources

```bash
kubectl-ai --llm-provider bedrock "create a deployment for redis with persistent storage"
```

### Debugging Issues

```bash
kubectl-ai --llm-provider bedrock "why is my pod stuck in pending state?"
```

### Resource Management

```bash
kubectl-ai --llm-provider bedrock "scale down all non-essential deployments"
```

### Security Analysis

```bash
kubectl-ai --llm-provider bedrock "audit my cluster for security issues"
```

## Support

For issues specific to the Bedrock integration:

1. Check the [kubectl-ai GitHub Issues](https://github.com/GoogleCloudPlatform/kubectl-ai/issues)
2. Review [AWS Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
3. Verify your AWS credentials and permissions
4. Enable debug mode for detailed error messages

## Contributing

The Bedrock integration is actively maintained. Contributions are welcome:

- Report bugs and issues
- Request new model support
- Improve documentation
- Add region-specific features

See our [Contributing Guide](../contributing.md) for more details. 