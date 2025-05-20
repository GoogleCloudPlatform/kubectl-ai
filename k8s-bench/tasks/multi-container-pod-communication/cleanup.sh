#!/usr/bin/env bash

kubectl delete pod communication-pod -n multi-container-test --ignore-not-found
kubectl delete configmap shared-data -n multi-container-test --ignore-not-found
kubectl delete namespace multi-container-test --ignore-not-found