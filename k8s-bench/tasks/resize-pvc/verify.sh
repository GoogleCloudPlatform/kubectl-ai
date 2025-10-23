#!/bin/bash

# Configuration
PVC_NAME="storage-pvc" 
EXPECTED_SIZE="15Gi"
TIMEOUT="120s"

echo "Attempting to get PV name from PVC: $PVC_NAME"

# Dynamically get the PV name from the PVC
PV_NAME=$(kubectl get pvc "$PVC_NAME" -n resize-pv -o jsonpath='{.spec.volumeName}')

if [ -z "$PV_NAME" ]; then
  echo "Error: Could not retrieve PersistentVolume name for PVC '$PVC_NAME'. Make sure the PVC exists and is bound."
  exit 1
fi

if ! kubectl wait --for=jsonpath='{.spec.capacity.storage}'='15Gi' pv/$PV_NAME --timeout=$TIMEOUT; then
  echo "FAILURE: PersistentVolume '$PV_NAME' did not reach the expected capacity of $EXPECTED_SIZE."
  exit 1 
else 
  echo "SUCCESS: PersistentVolume '$PV_NAME' reached the expected capacity of $EXPECTED_SIZE."
fi