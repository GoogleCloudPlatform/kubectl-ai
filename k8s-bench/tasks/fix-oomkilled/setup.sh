#!/usr/bin/env bash
kubectl delete namespace webapp-backend --ignore-not-found

# Create namespace
kubectl create namespace webapp-backend

# Apply the deployment from artifacts
kubectl apply -f artifacts/memory-hungry-app.yaml

# Wait for the deployment to be created and start experiencing OOMKilled events
kubectl rollout status deployment/backend-api -n webapp-backend --timeout=60s || true

# Give some time for the OOMKilled events to occur
sleep 10
