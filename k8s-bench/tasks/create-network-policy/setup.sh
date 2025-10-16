#!/usr/bin/env bash

# Cleanup existing namespaces if they exist
kubectl delete namespace ns1 --ignore-not-found
kubectl delete namespace ns2 --ignore-not-found

TIMEOUT="120s"

# Wait for namespaces to be fully deleted
echo "Waiting for namespaces to be fully deleted..."
while kubectl get namespace ns1 2>/dev/null || kubectl get namespace ns2 2>/dev/null; do
    sleep 1
done

# Create the namespaces
kubectl create namespace ns1
kubectl create namespace ns2

echo "Setup completed" 