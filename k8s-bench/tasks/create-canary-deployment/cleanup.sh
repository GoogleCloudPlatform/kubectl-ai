#!/bin/bash
set -e
NAMESPACE="canary-deployment-ns"

# Delete the namespace
kubectl delete namespace $NAMESPACE --wait=false --ignore-not-found
