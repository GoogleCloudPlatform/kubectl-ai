#!/usr/bin/env bash

echo "Setting up cluster state for dev environment eval..."

# Clean up any existing resources
kubectl delete namespace dev-alice dev-bob dev-charlie dev-shared staging prod --ignore-not-found=true

echo "Setup completed successfully"
