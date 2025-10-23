#!/usr/bin/env bash

NAMESPACE="webapp-backend"
DEPLOYMENT="backend-api"
ORIGINAL_MEMORY_LIMIT="128Mi"
TIMEOUT="120s"

# Check if the deployment is ready
if ! kubectl wait --for=condition=Available deployment/$DEPLOYMENT -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Deployment is not available"
    exit 1
fi

# Check if pods are running
if ! kubectl wait --for=condition=Ready pod -l app=backend-api -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Pods are not ready"
    exit 1
fi

# Check that the memory limit has been changed
MEMORY_LIMIT=$(kubectl get deployment/$DEPLOYMENT -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
if [ -z "$MEMORY_LIMIT" ]; then
    echo "Memory limit is not set"
    exit 1
fi

if [ $MEMORY_LIMIT == $ORIGINAL_MEMORY_LIMIT ]; then
    echo "Memory limit has not been changed from $ORIGINAL_MEMORY_LIMIT"
    exit 1
fi

# Check that there are no recent OOMKilled events
OOMKILLED_COUNT=$(kubectl get events -n $NAMESPACE --field-selector reason=OOMKilling --sort-by='.lastTimestamp' -o json | jq '.items | length')

if [ "$OOMKILLED_COUNT" -gt 0 ]; then
    # Check if the most recent OOMKilled event is from the last 2 minutes (indicating ongoing issues)
    RECENT_OOMKILLED=$(kubectl get events -n $NAMESPACE --field-selector reason=OOMKilling --sort-by='.lastTimestamp' -o jsonpath='{.items[-1].lastTimestamp}' 2>/dev/null)
    if [ -n "$RECENT_OOMKILLED" ]; then
        RECENT_TIME=$(date -d "$RECENT_OOMKILLED" +%s 2>/dev/null)
        CURRENT_TIME=$(date +%s)
        if [ $((CURRENT_TIME - RECENT_TIME)) -lt 120 ]; then
            echo "Recent OOMKilled events detected"
            exit 1
        fi
    fi
fi

echo "Pod is running successfully without OOMKilled events"
exit 0