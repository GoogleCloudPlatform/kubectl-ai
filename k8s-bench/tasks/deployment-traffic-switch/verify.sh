#!/bin/bash
set -e
NAMESPACE="e-commerce"
SERVICE_NAME="checkout-service"
EXPECTED_SELECTOR_VERSION="green"
TIMEOUT="120s"

echo "Waiting for the Service '$SERVICE_NAME' to point to version '$EXPECTED_SELECTOR_VERSION'..."
# Use 'kubectl wait' to verify the service selector condition
if ! kubectl wait --for=jsonpath='{.spec.selector.version}'="$EXPECTED_SELECTOR_VERSION" service/$SERVICE_NAME -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Failed to verify the service selector."
    exit 1
fi

echo "Service selector updated correctly."

echo "Verifying that service endpoints match the green deployment..."
# Use a single command to check if at least one endpoint has the desired label
kubectl get endpointslices -n $NAMESPACE -l kubernetes.io/service-name=$SERVICE_NAME \
  -o jsonpath='{.items[0].endpoints[*].conditions.ready}' | grep -q "true" || { echo "Failed to verify service endpoints."; exit 1; }

echo "Service endpoints correctly point to the green deployment."
echo "Verification successful!"
exit 0