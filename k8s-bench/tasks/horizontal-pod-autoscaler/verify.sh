#!/usr/bin/env bash
# Wait until HPA scales above 1 replica

HPA_NAME=$(kubectl get hpa -n hpa-test -o jsonpath='{.items[0].metadata.name}')
if [ -z "$HPA_NAME" ]; then
    echo "Error: No HPA found in hpa-test namespace."
    exit 1
fi

if kubectl wait "hpa/$HPA_NAME" -n hpa-test --for=condition=ScalingActive --timeout=120s; then
  exit 0
else
  echo "HPA did not scale above 1 replica in time"
  exit 1
fi
