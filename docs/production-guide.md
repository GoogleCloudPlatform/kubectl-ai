# Using kubectl-ai in Production Environments

This guide provides best practices and tips for safely using `kubectl-ai` in production environments.

## Security Best Practices

### 1. Run with Least Privilege

Always run `kubectl-ai` with the minimum required permissions:

```bash
# Create a dedicated service account with limited permissions
kubectl create serviceaccount kubectl-ai-sa

# Create a role with only the permissions you need
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubectl-ai-role
  namespace: default
rules:
- apiGroups: [""]
  resources: ["pods", "services", "deployments"]
  verbs: ["get", "list", "watch"]
EOF

# Bind the role to the service account
kubectl create rolebinding kubectl-ai-binding   --role=kubectl-ai-role   --serviceaccount=default:kubectl-ai-sa
```

### 2. Use Sandbox Environments

Test commands in a sandbox environment before running in production:

```bash
# Use a test namespace first
kubectl create namespace sandbox
kubectl config set-context --current --namespace=sandbox

# Test your commands in sandbox first
kubectl-ai "list all pods in the sandbox namespace"
```

### 3. Enable Command Approvals

Configure `kubectl-ai` to require approval before executing commands:

```bash
# Set environment variable for approval mode
export KUBECTL_AI_APPROVAL_MODE=always

# Or use the --approval flag
kubectl-ai --approval=always "delete all unused resources"
```

### 4. Use Enterprise LLM Providers

For production use, consider using enterprise-grade LLM providers with data privacy controls:

#### Using Vertex AI (Google Cloud)

```bash
# Configure Vertex AI as the LLM provider
export KUBECTL_AI_PROVIDER=vertexai
export KUBECTL_AI_PROJECT_ID=your-gcp-project
export KUBECTL_AI_LOCATION=us-central1

# Use with workload identity for secure authentication
kubectl-ai "scale deployment to 5 replicas"
```

#### Using Azure OpenAI

```bash
# Configure Azure OpenAI
export KUBECTL_AI_PROVIDER=azure-openai
export KUBECTL_AI_ENDPOINT=https://your-resource.openai.azure.com/
export KUBECTL_AI_API_KEY=your-api-key

kubectl-ai "check the status of all services"
```

## Operational Guidelines

### 1. Audit Logging

Enable audit logging to track all commands executed by `kubectl-ai`:

```bash
# Enable audit logging
export KUBECTL_AI_AUDIT_LOG=/var/log/kubectl-ai-audit.log

# All commands will be logged with timestamps
kubectl-ai "list all deployments"
```

### 2. Resource Quotas

Set resource quotas to prevent accidental resource exhaustion:

```bash
# Create resource quota for the namespace
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ResourceQuota
metadata:
  name: kubectl-ai-quota
  namespace: production
spec:
  hard:
    requests.cpu: "4"
    requests.memory: 8Gi
    limits.cpu: "8"
    limits.memory: 16Gi
EOF
```

### 3. Network Policies

Implement network policies to restrict `kubectl-ai` network access:

```bash
# Create network policy
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubectl-ai-network-policy
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: kubectl-ai
  policyTypes:
  - Ingress
  - Egress
  ingress: []
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
EOF
```

## Monitoring and Alerting

### 1. Monitor Command Execution

Set up monitoring for `kubectl-ai` command execution:

```bash
# Create a monitoring script
cat > /usr/local/bin/monitor-kubectl-ai.sh << 'EOF'
#!/bin/bash
while true; do
  timestamp=$(date '+%Y-%m-%d %H:%M:%S')
  commands=$(grep "kubectl-ai" /var/log/kubectl-ai-audit.log | wc -l)
  echo "[$timestamp] kubectl-ai commands executed: $commands"
  sleep 60
done
EOF

chmod +x /usr/local/bin/monitor-kubectl-ai.sh
```

### 2. Set Up Alerts

Configure alerts for suspicious activities:

```yaml
# Prometheus alert rules
groups:
  - name: kubectl-ai-alerts
    rules:
      - alert: HighCommandFrequency
        expr: rate(kubectl_ai_commands_total[5m]) > 10
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High frequency of kubectl-ai commands"
          
      - alert: UnauthorizedAccess
        expr: kubectl_ai_unauthorized_attempts > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Unauthorized kubectl-ai access attempt"
```

## Production Deployment Checklist

Before deploying `kubectl-ai` in production, ensure:

- [ ] **Service Account**: Created with least privilege permissions
- [ ] **Network Policies**: Implemented to restrict network access
- [ ] **Audit Logging**: Enabled and configured
- [ ] **Resource Quotas**: Set to prevent resource exhaustion
- [ ] **Approval Mode**: Enabled for command execution
- [ ] **LLM Provider**: Configured with enterprise controls
- [ ] **Monitoring**: Set up for command tracking
- [ ] **Alerting**: Configured for suspicious activities
- [ ] **Backup**: Regular backups of configuration and logs
- [ ] **Documentation**: Runbook for common scenarios

## Example Production Configuration

Here's a complete example configuration for production use:

```bash
# Production configuration
export KUBECTL_AI_PROVIDER=vertexai
export KUBECTL_AI_PROJECT_ID=your-production-project
export KUBECTL_AI_LOCATION=us-central1
export KUBECTL_AI_APPROVAL_MODE=always
export KUBECTL_AI_AUDIT_LOG=/var/log/kubectl-ai-audit.log
export KUBECTL_AI_LOG_LEVEL=info

# Use with kubeconfig for production cluster
export KUBECONFIG=/etc/kubernetes/production-kubeconfig.yaml

# Run with production settings
kubectl-ai "show me the status of all production deployments"
```

## Troubleshooting

### Common Issues

#### 1. Permission Denied

```bash
# Check service account permissions
kubectl auth can-i --list --as=system:serviceaccount:default:kubectl-ai-sa

# Verify role binding
kubectl get rolebinding kubectl-ai-binding -o yaml
```

#### 2. LLM Provider Connection Issues

```bash
# Test Vertex AI connection
gcloud auth application-default login
gcloud ai models list --region=us-central1

# Check API quotas
gcloud compute project-info describe --project=your-project
```

#### 3. Audit Log Not Writing

```bash
# Check log directory permissions
ls -la /var/log/

# Verify audit log configuration
echo $KUBECTL_AI_AUDIT_LOG

# Test log writing
kubectl-ai --version
tail -f /var/log/kubectl-ai-audit.log
```

## Support and Resources

- [kubectl-ai Documentation](https://github.com/GoogleCloudPlatform/kubectl-ai)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/overview/)
- [Google Cloud Security Controls](https://cloud.google.com/security/overview)

For issues and feature requests, please open an issue in the [GitHub repository](https://github.com/GoogleCloudPlatform/kubectl-ai/issues).
