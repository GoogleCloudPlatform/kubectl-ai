#!/usr/bin/env bash

# Check if the pod is in Running state with Ready status
echo "Checking if the pod is running and ready..."

TIMEOUT="120s"

# Wait up to TIMEOUT seconds for pod to become ready using kubectl wait
if kubectl wait --for=condition=Ready pod -l app=webapp -n orders --timeout=$TIMEOUT; then
  echo "Success: Pod is now Ready"
    
    # Check if probes exist at all
    LIVENESS_EXISTS=$(kubectl get deploy webapp -n orders -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}')
    READINESS_EXISTS=$(kubectl get deploy webapp -n orders -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}')
    
    if [ -z "$LIVENESS_EXISTS" ] || [ -z "$READINESS_EXISTS" ]; then
      echo "Failure: One or both probes have been removed completely."
      echo "Probes should be fixed, not removed."
      exit 1
    fi
    
    # Get the current probe configurations
    LIVENESS_PATH=$(kubectl get deploy webapp -n orders -o jsonpath='{.spec.template.spec.containers[0].livenessProbe.httpGet.path}')
    READINESS_PATH=$(kubectl get deploy webapp -n orders -o jsonpath='{.spec.template.spec.containers[0].readinessProbe.httpGet.path}')
    
    echo "Current liveness probe path: $LIVENESS_PATH"
    echo "Current readiness probe path: $READINESS_PATH"
    
    # Verify the probes are not using the nonexistent paths and have valid paths set
    if [ "$LIVENESS_PATH" != "/get_status" ] && [ "$READINESS_PATH" != "/is_ready" ] && \
       [ ! -z "$LIVENESS_PATH" ] && [ ! -z "$READINESS_PATH" ]; then
      echo "Success: Both probe paths have been fixed"
      
      # Check if pod is stable with no recent restarts
      RESTARTS=$(kubectl get pods -n orders -l app=webapp -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
      if [ "$RESTARTS" -lt 1 ]; then
        echo "Success: Pod is stable with acceptable number of restarts"
        exit 0
      else
        echo "Failure: Pod has too many restarts: $RESTARTS"
        exit 1
      fi
    else
      echo "Failure: One or both probe paths are still incorrect or missing:"
      echo "Liveness path: $LIVENESS_PATH"
      echo "Readiness path: $READINESS_PATH"
      exit 1
    fi
else
  echo "Failure: Pod is not Ready after waiting"
  kubectl get pods -n orders -l app=webapp
  exit 1
fi