#!/usr/bin/env bash
# Initialize namespace and deployment with the old image
kubectl delete namespace rollout-test --ignore-not-found
kubectl create namespace rollout-test
kubectl create deployment web-app --image=nginx:1.21 --replicas=3 -n rollout-test

# Wait until all replicas are available
TIMEOUT="120s"
if kubectl wait deployment/web-app -n rollout-test --for=condition=Available=True --timeout=$TIMEOUT; then
  echo "Setup succeeded for rolling-update-deployment"
  exit 0
else
  echo "Setup failed for rolling-update-deployment. Initial deployment did not become ready in time"
  exit 1
fi
