#!/usr/bin/env bash
set -e

# Delete namespace if it exists
kubectl delete namespace node-selector-test --ignore-not-found

# Create a fresh namespace
kubectl create namespace node-selector-test

# Apply the service and deployment with the invalid node selector
kubectl apply -f artifacts/service.yaml
kubectl apply -f artifacts/deployment.yaml

# Wait briefly to ensure resources are created
echo "Waiting for resources to be created..."
sleep 5

# Check the service has no endpoints (due to deployment with invalid node selector)
ENDPOINTS=$(kubectl get endpoints web-app-service -n node-selector-test -o jsonpath='{.subsets}')
if [[ -z "$ENDPOINTS" ]]; then
  echo "Setup successful: Service has no endpoints as expected"
else
  echo "Unexpected state: Service has endpoints"
  exit 1
fi
