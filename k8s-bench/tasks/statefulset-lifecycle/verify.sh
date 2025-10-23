#!/usr/bin/env bash
set -euo pipefail

# --- Configuration ---
NAMESPACE="statefulset-test"
STS_NAME="db"
EXPECTED_CONTENT="initial_data"

echo "Verifying old pods are deleted"
# Wait for scale-down: 1 ready pods and deletion of old pods
kubectl wait pod/db-1 pod/db-2 -n statefulset-test --for=delete --timeout=120s
echo "Old pods are deleted"

# Verify correct number of replicas
echo "Verifying StatefulSet replica count"
replicas=$(kubectl get sts "${STS_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.replicas}')
if [[ "${replicas}" -ne 1 ]]; then
  echo "Expected 1 replicas, but got $replicas"
  exit 1
fi
echo "StatefulSet is running with 1 replicas"

# Verify db-0 exists and have the correct data
for pod in db-0; do
  if ! kubectl get pod "$pod" -n "${NAMESPACE}" &> /dev/null; then
    echo "Pod $pod not found in namespace $NAMESPACE"
    exit 1
  fi

  data=$(kubectl exec "$pod" -n "${NAMESPACE}" -- cat /data/test)
  if [[ "$data" != "${EXPECTED_CONTENT}" ]]; then
    echo "Data missing or incorrect in $pod"
    exit 1
  fi
done

exit 0
