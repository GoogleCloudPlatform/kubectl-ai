#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

NAMESPACE=list-images-for-pods

kubectl delete namespace ${NAMESPACE} --ignore-not-found
kubectl create namespace ${NAMESPACE}

# Create artifacts
kubectl apply -f artifacts/manifest.yaml

# Wait for everything to be ready
# Can't wait for statefulset directly (sadly)
# Needs a new version of kubectl: kubectl wait --for=create --timeout=30s Pod/mysql-0 -n ${NAMESPACE}
sleep 5 # Wait for pod to be created (hopefully)
kubectl wait --for=condition=Ready --timeout=180s Pod/mysql-0 -n ${NAMESPACE}
