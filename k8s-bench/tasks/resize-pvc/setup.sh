#!/usr/bin/env bash
kubectl delete namespace resize-pv --ignore-not-found
kubectl create namespace resize-pv

kubectl apply -f artifacts/storage-pvc.yaml
kubectl apply -f artifacts/storage-pod.yaml