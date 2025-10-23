#!/usr/bin/env bash

# Delete namespace if exists and create a fresh one
kubectl delete namespace orders --ignore-not-found
kubectl create namespace orders

TIMEOUT="120s"

# Create a deployment with problematic health checks
cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp
  namespace: orders
spec:
  replicas: 1
  selector:
    matchLabels:
      app: webapp
  template:
    metadata:
      labels:
        app: webapp
    spec:
      containers:
      - name: webapp
        image: nginx:latest
        ports:
        - containerPort: 80
        # The problem: incorrect health probes causing restarts
        livenessProbe:
          httpGet:
            path: /get_status 
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 5
        readinessProbe:
          httpGet:
            path: /is_ready  
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 5
YAML

# Create a service for the webapp
kubectl create service clusterip webapp -n orders --tcp=80:80

# Wait for the pod to start and begin restarting due to failed probes
echo "Waiting for pod to start and begin failing health checks..."
if ! kubectl wait --for=condition=Available=False --timeout=$TIMEOUT deployment/webapp -n orders; then
    echo "Error: Timed out waiting for the deployment to become unavailable."
    exit 1
fi

echo "Setup successful: Deployment is no longer available."
exit 0