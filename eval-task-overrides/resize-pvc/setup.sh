#!/usr/bin/env bash
set -euo pipefail

kubectl delete namespace resize-pv --ignore-not-found
kubectl create namespace resize-pv

# Apply a self-contained expandable storage setup that works on any cluster
# (KinD, GKE, minikube, etc.) without relying on the default StorageClass.
kubectl apply -f artifacts/storage-class.yaml
kubectl apply -f artifacts/storage-pv.yaml
kubectl apply -f artifacts/storage-pvc.yaml
kubectl apply -f artifacts/storage-pod.yaml

kubectl wait --for=condition=Ready pod/storage-pod -n resize-pv --timeout=120s
