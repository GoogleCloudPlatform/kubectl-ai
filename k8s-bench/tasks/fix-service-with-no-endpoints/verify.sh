#!/usr/bin/env bash
set -e

# Check if the deployment exists
if ! kubectl get deployment web-app-deployment -n node-selector-test &>/dev/null; then
  echo "Deployment 'web-app-deployment' does not exist in namespace 'node-selector-test'"
  exit 1
fi

# Check if the invalid node selector has been removed or modified from the deployment
NODE_SELECTOR=$(kubectl get deployment web-app-deployment -n node-selector-test -o jsonpath='{.spec.template.spec.nodeSelector}')
if [[ "$NODE_SELECTOR" == *"nonexistent-label-key"* ]]; then
  echo "The invalid node selector is still present in the deployment"
  exit 1
fi

# Check if pods are being created successfully
echo "Waiting for pods to become ready..."
if ! kubectl wait --for=condition=Ready pods -l app=web-app -n node-selector-test --timeout=60s; then
  echo "Pods are not reaching Ready state after fixing the node selector"
  exit 1
fi

# Verify that the service now has endpoints
ENDPOINTS=$(kubectl get endpoints web-app-service -n node-selector-test -o jsonpath='{.subsets[0].addresses}')
if [[ -z "$ENDPOINTS" ]]; then
  echo "Service still has no endpoints after fixing the deployment"
  exit 1
fi
