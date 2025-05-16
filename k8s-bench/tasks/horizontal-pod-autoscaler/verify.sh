#!/usr/bin/env bash
# Wait until HPA scales above 1 replica
if kubectl wait hpa/web-app -n hpa-test --for=condition=ScalingActive --timeout=120s; then
  exit 0
else
  echo "HPA did not scale above 1 replica in time"
  exit 1
fi
