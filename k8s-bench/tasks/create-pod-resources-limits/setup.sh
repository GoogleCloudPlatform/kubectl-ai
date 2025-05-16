#!/usr/bin/env bash

# Delete the namespace if it exists to ensure a clean state
kubectl delete namespace limits-test --ignore-not-found