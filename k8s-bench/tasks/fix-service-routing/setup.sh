#!/usr/bin/env bash
# Create namespace and deployment with one set of labels
kubectl delete namespace web --ignore-not-found
kubectl create namespace web

# Create deployment with label app=nginx
kubectl create deployment nginx --image=nginx -n web
# kubectl label deployment nginx -n web app=nginx --overwrite

# Create service with different selector (app=web)
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: web
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: web  # Mismatched label - deployment has app=nginx
EOF

# Wait for deployment to be ready
for i in {1..30}; do
    if kubectl get deployment nginx -n web -o jsonpath='{.status.availableReplicas}' | grep -q "1"; then
        exit 0
    fi
    sleep 2
done 