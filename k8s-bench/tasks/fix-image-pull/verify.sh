#!/usr/bin/env bash
# Wait for pod to be ready
if kubectl wait --for=condition=Ready pod -l app=nginx -n debug --timeout=25s; then
    # Get current restart count
    restarts=$(kubectl get pods -n debug -l app=nginx -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
    
    # Wait additional 5 seconds to ensure stability
    sleep 5
    
    # Check if restart count hasn't increased
    new_restarts=$(kubectl get pods -n debug -l app=nginx -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')
    if [[ "$restarts" == "$new_restarts" ]]; then
        exit 0
    fi
fi

# If we get here, pod didn't stabilize in time
exit 1 