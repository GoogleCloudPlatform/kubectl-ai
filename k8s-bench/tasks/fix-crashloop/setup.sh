#!/usr/bin/env bash
kubectl delete namespace crashloop-test --ignore-not-found
# Create namespace and a deployment with an invalid command that will cause crashloop
kubectl create namespace crashloop-test
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: crashloop-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx
        command: ["/bin/sh", "-c"]
        args: ["python3 -c 'print('Starting'))'"] 
EOF

# Wait for pod to enter crashloop state
for i in {1..30}; do
    if kubectl get pods -n crashloop-test -l app=nginx -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}' | grep -q "[1-9]"; then
        exit 0
    fi
    sleep 1
done 