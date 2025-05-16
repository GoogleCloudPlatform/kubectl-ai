#!/usr/bin/env bash
# Wait for rollout to complete
kubectl rollout status deployment/web-app -n rollout-test --timeout=120s || exit 1

# Verify each pod is running the new image
pods=$(kubectl get pods -n rollout-test -l app=web-app -o jsonpath='{.items[*].spec.containers[0].image}')
for img in $pods; do
  if [[ "$img" != "nginx:1.22" ]]; then
    exit 1
  fi
done

exit 0
