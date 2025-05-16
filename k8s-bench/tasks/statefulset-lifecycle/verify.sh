#!/usr/bin/env bash

# Verify only db-0 and db-1 remain
for pod in db-0 db-1; do
  kubectl get pod "$pod" -n statefulset-test || exit 1
  data=$(kubectl exec "$pod" -n statefulset-test -- cat /data/test)
  if [[ "$data" != "test" ]]; then
    echo "Data missing or incorrect in $pod"
    exit 1
  fi
done
# Wait for scale-down: 2 ready pods and deletion of old pods
kubectl wait pod/db-2 pod/db-3 pod/db-4 -n statefulset-test --for=delete --timeout=120s

exit 0
