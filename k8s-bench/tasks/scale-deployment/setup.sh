#!/usr/bin/env bash
# Create namespace and a deployment with initial replicas
kubectl delete namespace scale-test --ignore-not-found
kubectl create namespace scale-test
kubectl create deployment web-app --image=nginx --replicas=1 -n scale-test
# Wait for initial deployment to be ready
for i in {1..30}; do
    if kubectl get deployment web-app -n scale-test -o jsonpath='{.status.availableReplicas}' | grep -q "1"; then
        exit 0
    fi
    sleep 2
done

echo "Setup failed for scale-deployment"
exit 1