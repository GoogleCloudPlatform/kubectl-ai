#!/usr/bin/env bash
kubectl delete namespace create-pod-test --ignore-not-found
kubectl create namespace create-pod-test