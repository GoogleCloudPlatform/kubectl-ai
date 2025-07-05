#!/usr/bin/env bash

echo "Cleaning up dev cluster eval resources..."

# Delete all created namespaces (this will clean up most resources)
kubectl delete namespace dev-alice dev-bob dev-charlie dev-shared staging prod --ignore-not-found=true

# Clean up any cluster-level RBAC resources that might have been created
kubectl delete clusterrole dev-* --ignore-not-found=true
kubectl delete clusterrolebinding dev-* --ignore-not-found=true

echo "Cleanup completed"
