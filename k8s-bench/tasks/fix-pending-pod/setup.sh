#!/usr/bin/env bash
kubectl delete namespace homepage-ns --ignore-not-found
kubectl create namespace homepage-ns

kubectl apply -f artifacts/homepage-pvc.yaml
kubectl apply -f artifacts/homepage-pod.yaml