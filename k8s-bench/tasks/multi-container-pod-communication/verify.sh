#!/usr/bin/env bash
set -e

# Wait for pod to be running
echo "Waiting for communication-pod to be ready..."
if ! kubectl wait --for=condition=Ready pod/communication-pod -n multi-container-test --timeout=60s; then
    echo "Pod failed to reach Ready state in time"
    echo "Current pod status:"
    kubectl describe pod communication-pod -n multi-container-test
    exit 1
fi

echo "Pod is ready. Verifying configuration..."

# then verify that both containers are running
CONTAINERS=$(kubectl get pod communication-pod -n multi-container-test -o jsonpath='{.spec.containers[*].name}')
if [[ ! "$CONTAINERS" == *"web"* ]] || [[ ! "$CONTAINERS" == *"logger"* ]]; then
    echo "Pod does not have both 'web' and 'logger' containers"
    exit 1
fi

# does the shared volume exists
VOLUMES=$(kubectl get pod communication-pod -n multi-container-test -o jsonpath='{.spec.volumes[*].name}')
if [[ ! "$VOLUMES" == *"logs-volume"* ]]; then
    echo "Pod does not have the required 'logs-volume' volume"
    exit 1
fi

# is web server accessible
echo "Testing web server access..."
kubectl exec communication-pod -n multi-container-test -c web -- curl -s -o /dev/null -w "%{http_code}" localhost:80 | grep -q 200

# logger container can see the access logs
echo "Verifying logger container can access nginx logs..."
kubectl exec communication-pod -n multi-container-test -c logger -- ls -la /var/log/nginx/access.log

echo "All verification checks passed!"
exit 0