#!/usr/bin/env bash

# Configuration
POD_NAME="homepage-pod"
NAMESPACE="homepage-ns"
PVC_NAME="homepage-pvc"
TIMEOUT="120s"
TASK_NAME="fix-pending-pods"

echo "Starting verification for $TASK_NAME..." 
# Verify the PersistentVolumeClaim is bound
echo "ℹWaiting for PVC '$PVC_NAME' to be 'Bound'..."
if ! kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc/$PVC_NAME -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "PVC '$PVC_NAME' did not become Bound within $TIMEOUT."
    echo "Info for '$PVC_NAME' in namespace '$NAMESPACE':"
    kubectl describe pvc $PVC_NAME -n $NAMESPACE
    echo "---"
    echo "Info for StorageClass and PersistentVolumes:"
    kubectl get sc,pv
    exit 1
fi
echo "'$PVC_NAME' is Bound. Verifying that desired state is realized..."

# Verify the Pod is Ready
echo "Waiting for Pod '$POD_NAME' to be 'Ready'..."
if ! kubectl wait --for=condition=Ready pod/$POD_NAME -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Pod '$POD_NAME' did not become Ready within $TIMEOUT."
    echo "---"
    echo "Info for Pod '$POD_NAME' in namespace '$NAMESPACE':"
    kubectl describe pod $POD_NAME -n $NAMESPACE
    exit 1
fi
echo "Pod '$POD_NAME' is Ready. Verification successful for $EVAL_NAME."
exit 0
