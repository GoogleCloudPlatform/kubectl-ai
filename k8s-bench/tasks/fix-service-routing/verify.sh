#!/usr/bin/env bash
# Check if service has endpoints
endpoints=$(kubectl get endpoints nginx -n web -o jsonpath='{.subsets[0].addresses}')
if [[ ! -z "$endpoints" ]]; then
    # Verify service can access the pod
    if kubectl run -n web test-connection --image=busybox --restart=Never --rm -i --wait --timeout=180s \
        -- wget -qO- nginx; then
        exit 0
    fi
fi

# If we get here, service connection failed
exit 1 