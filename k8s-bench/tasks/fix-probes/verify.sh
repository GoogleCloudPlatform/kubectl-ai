#!/bin/bash

# Check if the pod is in Running state with Ready status
echo "Checking if the pod is running and ready..."

# Wait up to 30 seconds for pod to become ready
for i in {1..15}; do
  READY=$(kubectl get pods -n health-check -l app=webapp -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
  if [ "$READY" == "true" ]; then
    echo "Success: Pod is now Ready"
    
    # Get the current probe configuration
    PROBE_PATH=$(kubectl get deploy webapp -n health-check -o jsonpath='{.spec.template.spec.containers[0].livenessProbe.httpGet.path}')
    
    echo "Current probe path: $PROBE_PATH"
    
    # Verify the probe is not using the nonexistent path
    if [ "$PROBE_PATH" != "/nonexistent-path" ]; then
      echo "Success: Probe path has been fixed"
      
      # Check if pod is stable with no recent restarts
      RESTARTS=$(kubectl get pods -n health-check -l app=webapp -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
      if [ "$RESTARTS" -lt 5 ]; then
        echo "Success: Pod is stable with acceptable number of restarts"
        exit 0
      else
        echo "Failure: Pod has too many restarts: $RESTARTS"
        exit 1
      fi
    else
      echo "Failure: Probe path is still incorrect"
      exit 1
    fi
  fi
  sleep 2
done

echo "Failure: Pod is not Ready after waiting"
kubectl get pods -n health-check -l app=webapp
exit 1
