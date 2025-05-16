#!/usr/bin/env bash
# Initialize namespace and deployment with CPU load generator
kubectl delete namespace hpa-test --ignore-not-found
kubectl create namespace hpa-test

# Create a Deployment with CPU request to allow HPA to target utilization
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: hpa-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-app
  template:
    metadata:
      labels:
        app: web-app
    spec:
      containers:
      - name: web-app
        image: busybox
        command: ["sh", "-c", "while true; do dd if=/dev/zero of=/dev/null; done"]
        resources:
          requests:
            cpu: "100m"
EOF
