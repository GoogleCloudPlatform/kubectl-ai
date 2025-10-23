#!/usr/bin/env bash
# Wait for deployment to scale down to 1 replicas with kubectl wait
TIMEOUT="120s"
if kubectl wait --for=condition=Available=True --timeout=$TIMEOUT deployment/web-service -n scale-down-test; then
    # Verify the replica count is exactly 1
    if [ "$(kubectl get deployment web-service -n scale-down-test -o jsonpath='{.status.availableReplicas}')" = "1" ]; then
        exit 0
    fi
fi

# If we get here, deployment didn't scale down correctly in time
echo "Verification failed for scale-down-deployment"
exit 1 