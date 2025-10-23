#!/usr/bin/env bash
kubectl delete namespace calc-app --ignore-not-found # clean up, just in case
kubectl create namespace calc-app
kubectl create configmap calc-app-map --from-file=artifacts/calc-app.py --namespace=calc-app
kubectl apply -f artifacts/calc-app-pod.yaml --namespace=calc-app

# Wait for pod to be ready
echo "Waiting for pod to be ready..."
TIMEOUT="120s"
kubectl wait --for=condition=Ready pod/calc-app-pod --namespace=calc-app --timeout=$TIMEOUT
