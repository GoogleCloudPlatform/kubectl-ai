#!/usr/bin/env bash
# Verify the HPA spec matches the desired state

HPA_NAME=$(kubectl get hpa -n hpa-test -o jsonpath='{.items[0].metadata.name}')
if [ -z "$HPA_NAME" ]; then
    echo "Error: No HPA found in hpa-test namespace."
    exit 1
fi

HPA_JSON=$(kubectl get hpa "$HPA_NAME" -n hpa-test -o json)

SCALE_TARGET_REF_NAME=$(echo "$HPA_JSON" | jq -r '.spec.scaleTargetRef.name')
MIN_REPLICAS=$(echo "$HPA_JSON" | jq -r '.spec.minReplicas')
MAX_REPLICAS=$(echo "$HPA_JSON" | jq -r '.spec.maxReplicas')
TARGET_CPU_UTILIZATION_API_V1=$(echo "$HPA_JSON" | jq -r '.spec.targetCPUUtilizationPercentage')
TARGET_CPU_UTILIZATION_API_V2=$(echo "$HPA_JSON" | jq -r '.spec.metrics[0].resource.target.averageUtilization')

if [ "$SCALE_TARGET_REF_NAME" != "web-app" ]; then
    echo "Verification failed: Expected HPA target to be 'web-app' deployment, got '$SCALE_TARGET_REF_NAME'."
    exit 1
fi

if [ "$MIN_REPLICAS" != "1" ]; then
    echo "Verification failed: Expected HPA minReplicas to be set to 1, got '$MIN_REPLICAS'."
    exit 1
fi

if [ "$MAX_REPLICAS" != "3" ]; then
    echo "Verification failed: Expected HPA maxReplicas to be set to 3, got '$MAX_REPLICAS'."
    exit 1
fi

if [ "$TARGET_CPU_UTILIZATION_API_V1" != "50" ] && [ "$TARGET_CPU_UTILIZATION_API_V2" != "50" ]; then
    echo "Verification failed: Expected HPA target CPU utilization to be set to 50 for either v1 or v2 API, got v1: '$TARGET_CPU_UTILIZATION_API_V1', v2: '$TARGET_CPU_UTILIZATION_API_V2'."
    exit 1
fi

echo "Waiting for HPA condition AbleToScale to be true..."
if ! kubectl wait --for=condition=AbleToScale=true hpa/"$HPA_NAME" -n hpa-test --timeout=120s; then
    echo "Verification failed: HPA condition AbleToScale did not become true within 120 seconds."
    exit 1
fi

echo "Successful verification: HPA spec is configured as expected."
exit 0
