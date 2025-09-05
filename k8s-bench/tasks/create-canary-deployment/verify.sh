#!/bin/bash
# A more robust script header
# exit on any error, exit on undeclared variable, and fail pipe commands on the first error
set -euo pipefail

# All variables are defined here for easy modification
NAMESPACE="canary-deployment-ns"
CANARY_DEPLOYMENT="engine-v2-1"
STABLE_DEPLOYMENT="engine-v2-0"
SERVICE="recommendation-engine"
CANARY_IMAGE="nginx:1.29"
CANARY_REPLICAS=1
STABLE_REPLICAS=10
EXPECTED_TOTAL_ENDPOINTS=$((CANARY_REPLICAS + STABLE_REPLICAS))
TIMEOUT="30s"

# Wait for both deployments to be available and fully rolled out
if ! kubectl wait deployment "$CANARY_DEPLOYMENT" -n "$NAMESPACE" \
  --for=condition=Available=true \
  --timeout="$TIMEOUT"; then
    echo "ERROR: Failed to find available canary deployment: '$CANARY_DEPLOYMENT'."
    exit 1
fi

if ! kubectl wait deployment "$CANARY_DEPLOYMENT" -n "$NAMESPACE" \
  --for=condition=Available=true \
  --for=jsonpath="{.status.updatedReplicas}=${CANARY_REPLICAS}" \
  --timeout="$TIMEOUT"; then

    echo "ERROR: Canary deployment does not have the correct amount of replicas."
    exit 1
fi


if ! kubectl wait deployment "$STABLE_DEPLOYMENT" -n "$NAMESPACE" \
  --for=condition=Available=true \
  --for=jsonpath="{.status.updatedReplicas}=${STABLE_REPLICAS}" \
  --timeout="$TIMEOUT"; then

    echo "ERROR: Stable deployment does not have the correct amount of replicas."
    exit 1
fi

# Verify the canary deployment is using the correct container image
CURRENT_CANARY_IMAGE=$(kubectl get deployment "$CANARY_DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}')
if [[ "$CURRENT_CANARY_IMAGE" != "$CANARY_IMAGE" ]]; then
  echo "ERROR: Canary deployment image is '$CURRENT_CANARY_IMAGE', expected '$CANARY_IMAGE'."
  exit 1
fi

# Verify the service selector targets the general app label without a version
SERVICE_JSON=$(kubectl get svc "$SERVICE" -n "$NAMESPACE" -o json)
SELECTOR_APP=$(echo "$SERVICE_JSON" | jq -r '.spec.selector.app')
# 'jq' will output the string 'null' if the key doesn't exist
SELECTOR_VERSION=$(echo "$SERVICE_JSON" | jq -r '.spec.selector.version')

if [[ "$SELECTOR_APP" != "$SERVICE" ]] || [[ "$SELECTOR_VERSION" != "null" ]]; then
    echo "ERROR: Service selector is incorrect. It should only target 'app: $SERVICE'."
    echo "   Found: app='$SELECTOR_APP', version='$SELECTOR_VERSION'"
    exit 1
fi

# Wait until the service has endpoints for all pods (stable + canary) to confirm traffic is flowing to all expected pods
echo "--> Waiting for service '$SERVICE' to have '$EXPECTED_TOTAL_ENDPOINTS' endpoints..."
success=false
# This loop will try 18 times, waiting 5 seconds between each attempt (90s total timeout)
for i in {1..18}; do
    # Using jq for a readable and robust way to count ready addresses.
    count=$(kubectl get endpoints "$SERVICE" -n "$NAMESPACE" -o json 2>/dev/null | jq '([.subsets[].addresses // [] | length] | add) // 0' || echo 0)

    if [[ "$count" -eq "$EXPECTED_TOTAL_ENDPOINTS" ]]; then
        echo "Success: Service has all $count endpoints."
        success=true
        break
    fi

    echo "  ($i/18) Waiting... found $count endpoints. Retrying in 5 seconds..."
    sleep 5
done

if ! $success; then
    echo "ERROR: Timed out waiting for service endpoints."
    echo "  Found $count endpoints, but expected $EXPECTED_TOTAL_ENDPOINTS."
    exit 1
fi


echo "--- Verification Successful! ---"
echo "Canary deployment is correctly configured and receiving traffic."
exit 0