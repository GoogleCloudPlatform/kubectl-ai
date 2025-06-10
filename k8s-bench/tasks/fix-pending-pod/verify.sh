#!/usr/bin/env bash

# Configuration
POD_NAME="homepage-pod"
NAMESPACE="homepage-ns" 

if ! kubectl wait --for=condition=Ready pod/$POD_NAME -n $NAMESPACE --timeout=30s; then
  exit 1 
else 
  exit 0
fi