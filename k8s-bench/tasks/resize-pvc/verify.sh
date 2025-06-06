#!/bin/bash

# Configuration
PVC_NAME="storage-pvc" 
EXPECTED_SIZE="15Gi"
RETRY_ATTEMPTS=10 
RETRY_INTERVAL_SECONDS=15 

echo "Attempting to get PV name from PVC: $PVC_NAME"

# Dynamically get the PV name from the PVC
PV_NAME=$(kubectl get pvc "$PVC_NAME" -n storage-test -o jsonpath='{.spec.volumeName}')

if [ -z "$PV_NAME" ]; then
  echo "Error: Could not retrieve PersistentVolume name for PVC '$PVC_NAME'. Make sure the PVC exists and is bound."
  exit 1
fi

echo "Found PersistentVolume '$PV_NAME' for PVC '$PVC_NAME'."
echo "Verifying size of PersistentVolume: $PV_NAME to be $EXPECTED_SIZE"

for i in $(seq 1 $RETRY_ATTEMPTS); do
  echo "Attempt $i of $RETRY_ATTEMPTS..."

  PV_CAPACITY=$(kubectl get pv "$PV_NAME" -o jsonpath='{.spec.capacity.storage}')

  if [ -n "$PV_CAPACITY" ]; then
    if [ "$PV_CAPACITY" == "$EXPECTED_SIZE" ]; then
      echo "SUCCESS: PersistentVolume '$PV_NAME' capacity is now: $PV_CAPACITY"
      exit 0 # Success, exit the script
    else
      echo "Current capacity for PV '$PV_NAME' is $PV_CAPACITY. Expected: $EXPECTED_SIZE. Retrying in $RETRY_INTERVAL_SECONDS seconds..."
      sleep $RETRY_INTERVAL_SECONDS
    fi
  else
    echo "Capacity not yet available for PV '$PV_NAME'. Retrying in $RETRY_INTERVAL_SECONDS seconds..."
    sleep $RETRY_INTERVAL_SECONDS
  fi
done

echo "FAILURE: PersistentVolume '$PV_NAME' did not reach the expected capacity of $EXPECTED_SIZE after $RETRY_ATTEMPTS attempts. Current size: $PV_CAPACITY"
exit 1 # Failure, exit with an error code