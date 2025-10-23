#!/usr/bin/env bash
set -e

# Check if the deployment exists
if ! kubectl get deployment web-app-deployment -n webshop-frontend &>/dev/null; then
  echo "Deployment 'web-app-deployment' does not exist in namespace 'webshop-frontend'"
  exit 1
fi

# Check if pods are being created successfully
echo "Waiting for pods to become ready..."
TIMEOUT="120s"
if ! kubectl wait --for=condition=Ready pods -l app=web-app -n webshop-frontend --timeout=$TIMEOUT; then
  echo "Pods are not reaching Ready state after fixing the node selector"
  exit 1
fi

# Verify that the service now has endpoints
ENDPOINTS=$(kubectl get endpoints web-app-service -n webshop-frontend -o jsonpath='{.subsets[0].addresses}')
if [[ -z "$ENDPOINTS" ]]; then
  echo "Service still has no endpoints after fixing the deployment"
  exit 1
fi
