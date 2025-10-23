#!/usr/bin/env bash

NAMESPACE="color-size-settings"
TIMEOUT="120s"

# Check if namespace exists
if ! kubectl get namespace $NAMESPACE &>/dev/null; then
    echo "Namespace '$NAMESPACE' does not exist"
    exit 1
fi

# Check if configmaps exist
if ! kubectl get configmap color-settings -n $NAMESPACE &>/dev/null; then
    echo "ConfigMap 'color-settings' does not exist in namespace '$NAMESPACE'"
    exit 1
fi

if ! kubectl get configmap size-settings -n $NAMESPACE &>/dev/null; then
    echo "ConfigMap 'size-settings' does not exist in namespace '$NAMESPACE'"
    exit 1
fi

# Check configmap contents
COLOR_VALUE=$(kubectl get configmap color-settings -n $NAMESPACE -o jsonpath='{.data.color}')
if [[ "$COLOR_VALUE" != "blue" ]]; then
    echo "ConfigMap 'color-settings' has incorrect value for key 'color': '$COLOR_VALUE', expected: 'blue'"
    exit 1
fi

SIZE_VALUE=$(kubectl get configmap size-settings -n $NAMESPACE -o jsonpath='{.data.size}')
if [[ "$SIZE_VALUE" != "medium" ]]; then
    echo "ConfigMap 'size-settings' has incorrect value for key 'size': '$SIZE_VALUE', expected: 'medium'"
    exit 1
fi

# Wait for pod to be ready
if ! kubectl wait --for=condition=Ready pod/pod1 -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Pod 'pod1' is not ready in namespace '$NAMESPACE'"
    exit 1
fi

# Verify pod has the correct image
POD_IMAGE=$(kubectl get pod pod1 -n $NAMESPACE -o jsonpath='{.spec.containers[0].image}')
if [[ "$POD_IMAGE" != "nginx:alpine" ]]; then
    echo "Pod has incorrect image: $POD_IMAGE, expected: nginx:alpine"
    exit 1
fi

# Verify the values are accessible in the pod
echo "Verifying environment variable in pod..."
ENV_TEST=$(kubectl exec pod1 -n $NAMESPACE -- sh -c 'echo $COLOR')
if [[ "$ENV_TEST" != "blue" ]]; then
    echo "Environment variable 'COLOR' is not accessible in the pod or has incorrect value: '$ENV_TEST'"
    exit 1
fi

echo "Verifying volume mount in pod..."
VOLUME_TEST=$(kubectl exec pod1 -n $NAMESPACE -- cat /etc/sizes/size)
if [[ "$VOLUME_TEST" != "medium" ]]; then
    echo "Volume mount is not accessible in the pod or file has incorrect content: '$VOLUME_TEST'"
    exit 1
fi

# All verifications passed
echo "All verifications passed!"
exit 0 