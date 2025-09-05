#!/bin/bash
set -e
NAMESPACE="canary-deployment-ns"

# Create the namespace
kubectl create namespace $NAMESPACE

# Apply the initial stable deployment and the service pointing only to it
kubectl apply -n $NAMESPACE -f artifacts/deployment-v1.yaml
kubectl apply -n $NAMESPACE -f artifacts/service.yaml
