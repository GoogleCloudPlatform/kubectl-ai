#!/usr/bin/env bash
kubectl delete namespace app-level-fails-test --ignore-not-found # clean up, just in case
kubectl create namespace app-level-fails-test
kubectl create configmap eval-app-map --from-file=artifacts/eval-app.py --namespace=app-level-fails-test
kubectl apply -f artifacts/eval-app-pod.yaml --namespace=app-level-fails-test

# Wait a few seconds for pod to get at least 1 failure logged
for i in {1..5}; do
    sleep 1
done
