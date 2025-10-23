#!/usr/bin/env bash
TIMEOUT="120s"

# Wait for the deployment rollout to complete and become "Available"
if kubectl wait --for=condition=Available deployment/app -n debug --timeout=$TIMEOUT; then
    # Get the restart count *only* from the new, running pod.
    restarts=$(kubectl get pods -n debug -l app=nginx --field-selector=status.phase=Running -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
    
    # Wait additional 5 seconds to ensure stability
    sleep 5
    
    # Check if restart count hasn't increased
    new_restarts=$(kubectl get pods -n debug -l app=nginx --field-selector=status.phase=Running -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
    if [[ "$restarts" == "$new_restarts" ]]; then
        echo "Pod is stable. Verification successful."
        exit 0
    else
        echo "Verification failed: Pod restarted unexpectedly."
        exit 1
    fi
fi

# If we get here, the deployment never became available
echo "Verification failed: Deployment 'app' did not become Available in $TIMEOUT."
exit 1