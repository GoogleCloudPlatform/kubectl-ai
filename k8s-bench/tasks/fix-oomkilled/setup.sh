#!/usr/bin/env bash
kubectl delete namespace webapp-backend --ignore-not-found

# Create namespace
kubectl create namespace webapp-backend

# Apply the deployment from artifacts
kubectl apply -f artifacts/memory-hungry-app.yaml

# Wait for the deployment to be created
kubectl rollout status deployment/backend-api -n webapp-backend --timeout=60s || true

# Wait until an OOMKilled event is detected (timeout after 30s)
echo "Waiting for OOMKilled event to occur..."
for i in {1..15}; do
  OOMKILLED_COUNT=$(kubectl get events -n webapp-backend --field-selector reason=OOMKilling -o json | jq '.items | length')
  if [ "$OOMKILLED_COUNT" -gt 0 ]; then
    echo "OOMKilled event detected."
    break
  fi
  sleep 2
done
