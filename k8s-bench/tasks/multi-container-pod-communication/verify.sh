#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="multi-container-logging"
POD_NAME="communication-pod"
TIMEOUT="120s"

# Wait for pod to be running
echo "Waiting for pod '$POD_NAME' to be ready..."
if ! kubectl wait --for=condition=Ready pod/$POD_NAME -n "$NAMESPACE" --timeout=$TIMEOUT; then
    echo "Pod failed to reach Ready state in time"
    echo "Current pod status:"
    kubectl describe pod "$POD_NAME" -n "$NAMESPACE"
    exit 1
fi

echo "Pod is ready. Verifying configuration..."

# then verify that both containers are running
CONTAINERS=$(kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.containers[*].name}')
if [[ ! "$CONTAINERS" == *"web-server"* ]] || [[ ! "$CONTAINERS" == *"logger"* ]]; then
    echo "Pod does not have both 'web-server' and 'logger' containers"
    exit 1
fi

# does the shared volume exists
VOLUMES=$(kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.volumes[*].name}')
if [[ ! "$VOLUMES" == *"logs-volume"* ]]; then
    echo "Pod does not have the required 'logs-volume' volume"
    exit 1
fi

# is web server accessible
echo "Testing web server access..."
kubectl exec "$POD_NAME" -n "$NAMESPACE" -c web-server -- curl -s -o /dev/null -w "%{http_code}" localhost:80 | grep -q 200

# logger container can see the access logs
echo "Verifying logger container can access nginx logs..."
kubectl exec "$POD_NAME" -n "$NAMESPACE" -c logger -- ls -la /var/log/nginx/access.log

echo "All verification checks passed!"
exit 0