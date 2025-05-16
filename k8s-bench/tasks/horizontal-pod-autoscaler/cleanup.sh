#!/usr/bin/env bash
# Tear down namespace and HPA resources
kubectl delete namespace hpa-test --ignore-not-found
exit 0
