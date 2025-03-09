#!/bin/bash
# Wait up to 30 seconds for service endpoints to be created
for i in {1..30}; do
    # Check if service has endpoints
    endpoints=$(kubectl get endpoints nginx -n web -o jsonpath='{.subsets[0].addresses}')
    if [[ ! -z "$endpoints" ]]; then
        # Verify service can access the pod
        if timeout 10 kubectl run -n web test-connection --image=busybox --restart=Never --rm -i  \
            -- wget -qO- nginx; then
            exit 0
        fi
    fi
    sleep 1
done

# If we get here, service connection failed
exit 1 