#!/usr/bin/env bash
kubectl delete namespace resize-pv --ignore-not-found
kubectl create namespace resize-pv

kubectl apply -f ./storage-pvc.yaml
kubectl apply -f ./storage-pod.yaml