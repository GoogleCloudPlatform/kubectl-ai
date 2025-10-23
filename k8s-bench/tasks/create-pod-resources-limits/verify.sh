#!/usr/bin/env bash

# Check if namespace exists
if ! kubectl get namespace limits-test &>/dev/null; then
    echo "Namespace 'limits-test' does not exist"
    exit 1
fi

# Wait for pod to be ready
TIMEOUT="120s"
if ! kubectl wait --for=condition=Ready pod/resource-limits-pod -n limits-test --timeout=$TIMEOUT; then
    echo "Pod 'resource-limits-pod' is not ready in namespace 'limits-test'"
    exit 1
fi

# Verify the pod has the correct image
POD_IMAGE=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].image}')
if [[ "$POD_IMAGE" != "httpd:alpine" ]]; then
    echo "Pod has incorrect image: $POD_IMAGE, expected: httpd:alpine"
    exit 1
fi

# Verify the container name
CONTAINER_NAME=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].name}')
if [[ "$CONTAINER_NAME" != "my-container" ]]; then
    echo "Container has incorrect name: $CONTAINER_NAME, expected: my-container"
    exit 1
fi

# Verify CPU request
CPU_REQUEST=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].resources.requests.cpu}')
if [[ "$CPU_REQUEST" != "60m" ]]; then
    echo "Container has incorrect CPU request: $CPU_REQUEST, expected: 60m"
    exit 1
fi

# Verify CPU limit
CPU_LIMIT=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].resources.limits.cpu}')
if [[ "$CPU_LIMIT" != "600m" ]]; then
    echo "Container has incorrect CPU limit: $CPU_LIMIT, expected: 600m"
    exit 1
fi

# Verify memory request
MEMORY_REQUEST=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].resources.requests.memory}')
if [[ "$MEMORY_REQUEST" != "62Mi" ]]; then
    echo "Container has incorrect memory request: $MEMORY_REQUEST, expected: 62Mi"
    exit 1
fi

# Verify memory limit
MEMORY_LIMIT=$(kubectl get pod resource-limits-pod -n limits-test -o jsonpath='{.spec.containers[0].resources.limits.memory}')
if [[ "$MEMORY_LIMIT" != "62Mi" ]]; then
    echo "Container has incorrect memory limit: $MEMORY_LIMIT, expected: 62Mi"
    exit 1
fi

# All verifications passed
echo "All verifications passed!"
exit 0 