#!/usr/bin/env bash
# Tear down namespace and deployment
kubectl delete namespace rollout-test --ignore-not-found
exit 0
