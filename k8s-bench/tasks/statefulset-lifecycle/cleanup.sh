#!/usr/bin/env bash
# Tear down namespace and StatefulSet resources
kubectl delete namespace statefulset-test --ignore-not-found
exit 0
