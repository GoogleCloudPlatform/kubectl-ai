#!/usr/bin/env bash
# Wait for pod to be running with kubectl wait
if kubectl wait --for=condition=Ready pod/web-server -n create-pod-test --timeout=120s; then
    exit 0
else
    # If we get here, pod didn't reach Running state in time
    exit 1
fi 