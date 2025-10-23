#!/usr/bin/env bash
set -e

# Delete namespace if it exists
kubectl delete namespace webshop-frontend --ignore-not-found

# Create a fresh namespace
kubectl create namespace webshop-frontend

# Apply the service and deployment with the invalid node selector
kubectl apply -f artifacts/service.yaml
kubectl apply -f artifacts/deployment.yaml

# Wait for the deployment to be available or timeout after 120 seconds
echo "Waiting for resources to be created..."
TIMEOUT="120s"
kubectl wait --for=condition=Available=False --timeout=$TIMEOUT deployment/web-app-deployment -n webshop-frontend || true

# Check the service has no endpoints (due to deployment with invalid node selector)
ENDPOINTS=$(kubectl get endpoints web-app-service -n webshop-frontend -o jsonpath='{.subsets}')
if [[ -z "$ENDPOINTS" ]]; then
  echo "Setup successful: Service has no endpoints as expected"
else
  echo "Unexpected state: Service has endpoints"
  exit 1
fi
