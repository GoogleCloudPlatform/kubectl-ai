#!/usr/bin/env bash

NAMESPACE="webapp-backend"
DEPLOYMENT="backend-api"
TIMEOUT="120s"

kubectl delete namespace $NAMESPACE --ignore-not-found

# Create namespace
kubectl create namespace $NAMESPACE

# Apply the deployment from artifacts
kubectl apply -f artifacts/memory-hungry-app.yaml

# Wait for the deployment to be created
kubectl rollout status deployment/backend-api -n webapp-backend --timeout=$TIMEOUT || true

# Wait until an OOMKilled event is detected (timeout after 30s)
echo "Waiting for OOMKilled event to occur..."
for i in {1..30}; do
  OOMKILLED_COUNT=$(kubectl get events -n webapp-backend --field-selector reason=OOMKilling -o json | jq '.items | length')
  if [ "$OOMKILLED_COUNT" -gt 0 ]; then
    echo "OOMKilled event detected."
    exit 0
    break
  fi
  sleep 2
done

echo "Failed to detect OOMKilled event. Kubectl events:"
kubectl get events -n $NAMESPACE
exit 1
