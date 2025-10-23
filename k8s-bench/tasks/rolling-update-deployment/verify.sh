#!/usr/bin/env bash
# Configuration constants
NAMESPACE="rollout-test"
DEPLOYMENT="web-app"
EXPECTED_IMAGE="nginx:1.22"
TIMEOUT="120s"
TASK_NAME="rolling-update-deployment"

echo "Starting verification for $TASK_NAME..."
# Wait for the rollout to complete
echo "Waiting for deployment '$DEPLOYMENT' in namespace '$NAMESPACE' to complete its rollout..."
if ! kubectl rollout status deployment/$DEPLOYMENT -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "ERROR: Deployment rollout failed or timed out after $TIMEOUT."
    exit 1
fi
echo "Deployment rollout completed successfully."

# Verify each pod is running the new image
echo "Verifying container images for all pods managed by the deployment..."
FAILURE=0

# Get the pod-template-hash from the new, active ReplicaSet
# (The one that has a desired replica count greater than 0)
ACTIVE_POD_HASH=$(kubectl get rs -n $NAMESPACE -l app=$DEPLOYMENT -o jsonpath='{.items[?(@.spec.replicas > 0)].metadata.labels.pod-template-hash}')

if [ -z "$ACTIVE_POD_HASH" ]; then
    echo "ERROR: Could not find active ReplicaSet hash for deployment '$DEPLOYMENT'."
    exit 1
fi

echo "Found active pod-template-hash: $ACTIVE_POD_HASH. Verifying pods with this label..."

# Get a list of pod names and images *only* from the active ReplicaSet
POD_INFO=$(kubectl get pods -n $NAMESPACE -l app=$DEPLOYMENT,pod-template-hash=$ACTIVE_POD_HASH -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.containers[0].image}{"\n"}{end}')

if [ -z "$POD_INFO" ]; then
    echo "ERROR: Could not find any pods for deployment '$DEPLOYMENT' with hash '$ACTIVE_POD_HASH'."
fi

# Loop through each line of the pod info
while read -r POD_NAME ACTUAL_IMAGE; do
    if [[ "$ACTUAL_IMAGE" == "$EXPECTED_IMAGE" ]]; then
        echo "PASSED: Pod '$POD_NAME' is running the correct image ($ACTUAL_IMAGE)."
    else
        echo "FAILED: Pod '$POD_NAME' has the wrong image. Expected: $EXPECTED_IMAGE, Found: $ACTUAL_IMAGE"
        FAILURE=1
    fi
done <<< "$POD_INFO"

if [ $FAILURE -eq 1 ]; then
    echo "Verification failed: One or more pods are not running the correct image."
    exit 1
else 
  echo "Verification successful for $TASK_NAME."
  exit 0
fi