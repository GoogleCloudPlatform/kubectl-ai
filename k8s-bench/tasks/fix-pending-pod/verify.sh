#!/usr/bin/env bash

# Configuration
POD_NAME="homepage-pod"
NAMESPACE="homepage-ns" 
EXPECTED_STATUS="Running"
RETRY_ATTEMPTS=10 
RETRY_INTERVAL_SECONDS=5

echo "Verifying Pod '$POD_NAME' in namespace '$NAMESPACE' is in '$EXPECTED_STATUS' status..."

for i in $(seq 1 $RETRY_ATTEMPTS); do
  echo "Attempt $i of $RETRY_ATTEMPTS..."

  # Get the current status of the pod
  CURRENT_STATUS=$(kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null) 

  if [ -z "$CURRENT_STATUS" ]; then
    echo "Pod '$POD_NAME' not found or status not yet available. Retrying in $RETRY_INTERVAL_SECONDS seconds..."
    sleep $RETRY_INTERVAL_SECONDS
    continue 
  fi

  if [ "$CURRENT_STATUS" == "Pending" ]; then
    echo "FAILURE: Pod '$POD_NAME' is in a '$CURRENT_STATUS' state. Please check 'kubectl describe pod $POD_NAME -n $NAMESPACE' for details."
    exit 1 # Exit with failure
  elif [ "$CURRENT_STATUS" == "$EXPECTED_STATUS" ]; then
    echo "SUCCESS: Pod '$POD_NAME' is in '$EXPECTED_STATUS' status."
    exit 0 # Exit with success
  elif [ "$CURRENT_STATUS" == "ContainerCreating" ] || [ "$CURRENT_STATUS" == "PodInitializing" ]; then
    echo "Pod '$POD_NAME' is currently '$CURRENT_STATUS'. Waiting for '$EXPECTED_STATUS'. Retrying in $RETRY_INTERVAL_SECONDS seconds..."
    sleep $RETRY_INTERVAL_SECONDS
  elif [ "$CURRENT_STATUS" == "Error" ] || [ "$CURRENT_STATUS" == "Failed" ] || [ "$CURRENT_STATUS" == "CrashLoopBackOff" ] || [ "$CURRENT_STATUS" == "ImagePullBackOff" ]; then
    echo "FAILURE: Pod '$POD_NAME' is in a '$CURRENT_STATUS' state. Please check 'kubectl describe pod $POD_NAME -n $NAMESPACE' for details."
    exit 1 # Exit with failure
  else
    echo "Pod '$POD_NAME' is in an unexpected '$CURRENT_STATUS' state. Retrying in $RETRY_INTERVAL_SECONDS seconds..."
    sleep $RETRY_INTERVAL_SECONDS
  fi
done

echo "FAILURE: Pod '$POD_NAME' did not reach '$EXPECTED_STATUS' status after $RETRY_ATTEMPTS attempts. Current status: $CURRENT_STATUS"
exit 1 # Exit with failure