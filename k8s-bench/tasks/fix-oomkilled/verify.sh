#!/usr/bin/env bash

NAMESPACE="webapp-backend"
DEPLOYMENT="backend-api"

# Check if the deployment is ready
if ! kubectl wait --for=condition=Available deployment/$DEPLOYMENT -n $NAMESPACE --timeout=60s; then
    echo "Deployment is not available"
    exit 1
fi

# Check if pods are running
if ! kubectl wait --for=condition=Ready pod -l app=backend-api -n $NAMESPACE --timeout=30s; then
    echo "Pods are not ready"
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
