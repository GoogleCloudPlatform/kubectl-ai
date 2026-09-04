#!/usr/bin/env bash
set -euo pipefail

# Configuration
PVC_NAME="storage-pvc"
NAMESPACE="resize-pv"
EXPECTED_SIZE="15Gi"

echo "Verifying PVC '$PVC_NAME' was resized to $EXPECTED_SIZE..."

ACTUAL_SIZE=$(kubectl get pvc "$PVC_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.resources.requests.storage}')

if [ -z "$ACTUAL_SIZE" ]; then
  echo "Error: Could not read requested storage size for PVC '$PVC_NAME'."
  kubectl describe pvc "$PVC_NAME" -n "$NAMESPACE"
  exit 1
fi

if [ "$ACTUAL_SIZE" != "$EXPECTED_SIZE" ]; then
  echo "FAILURE: PVC '$PVC_NAME' request size is '$ACTUAL_SIZE', expected '$EXPECTED_SIZE'."
  kubectl get pvc "$PVC_NAME" -n "$NAMESPACE" -o yaml
  exit 1
fi

echo "SUCCESS: PVC '$PVC_NAME' requested storage was updated to $EXPECTED_SIZE."

# If the cluster supports volume expansion, confirm the status reflects the new size.
STATUS_SIZE=$(kubectl get pvc "$PVC_NAME" -n "$NAMESPACE" -o jsonpath='{.status.capacity.storage}' 2>/dev/null || true)
if [ -n "$STATUS_SIZE" ] && [ "$STATUS_SIZE" = "$EXPECTED_SIZE" ]; then
  echo "SUCCESS: PVC status capacity also reports $EXPECTED_SIZE."
elif [ -n "$STATUS_SIZE" ]; then
  echo "NOTE: PVC status capacity is '$STATUS_SIZE'. Spec resize succeeded; backend expansion may be cluster-dependent."
fi
