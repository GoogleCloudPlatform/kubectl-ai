#!/usr/bin/env bash
# Wait for deployment to scale down to 2 replicas with kubectl wait
if kubectl wait --for=condition=Available=True --timeout=30s deployment/web-service -n scale-down-test; then
    # Verify the replica count is exactly 2
    if [ "$(kubectl get deployment web-service -n scale-down-test -o jsonpath='{.status.availableReplicas}')" = "2" ]; then
        exit 0
    fi
fi

# If we get here, deployment didn't scale down correctly in time
exit 1 