#!/usr/bin/env bash

NAMESPACE="web-server"
POD="web-server"

# Wait for pod to be running with kubectl wait
if ! kubectl wait --for=condition=Ready pod/$POD -n $NAMESPACE --timeout=120s; then
    echo "Pod $POD did not become Ready in time."
    exit 1
fi

IMAGE=$(kubectl get pod $POD -n $NAMESPACE -o jsonpath='{.spec.containers[0].image}')
if [ "$IMAGE" != "nginx" ]; then
    echo "Pod is using incorrect image: $IMAGE"
    exit 1
fi

echo "Success for create-pod. Pod is ready and using correct image."
exit 0