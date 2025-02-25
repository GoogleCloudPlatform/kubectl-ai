#!/bin/bash
# Create namespace and a deployment with initial replicas
kubectl create namespace scale-down-test
kubectl create deployment web-service --image=nginx --replicas=4 -n scale-down-test
# Wait for initial deployment to be ready
for i in {1..30}; do
    if kubectl get deployment web-service -n scale-down-test -o jsonpath='{.status.availableReplicas}' | grep -q "4"; then
        exit 0
    fi
    sleep 1
done 