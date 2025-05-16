#!/usr/bin/env bash
# Teardown any existing namespace
kubectl delete namespace statefulset-test --ignore-not-found

# Create namespace
kubectl create namespace statefulset-test
