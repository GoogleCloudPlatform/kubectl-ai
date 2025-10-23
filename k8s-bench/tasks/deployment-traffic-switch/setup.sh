#!/bin/bash
set -e
NAMESPACE="e-commerce"
TIMEOUT="120s"

# Create the namespace if it doesn't exist to make the script idempotent
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Apply all resource manifests from the artifacts directory
echo "Applying Kubernetes resources from artifacts/ directory..."
kubectl apply -n $NAMESPACE -f artifacts/

# Wait for both deployments to be available to ensure a stable starting state
echo "Waiting for blue deployment to be ready..."
kubectl rollout status deployment/checkout-service-blue -n $NAMESPACE --timeout=$TIMEOUT

echo "Waiting for green deployment to be ready..."
kubectl rollout status deployment/checkout-service-green -n $NAMESPACE --timeout=$TIMEOUT

echo "Setup complete. Service 'checkout-service' is pointing to 'blue'."
