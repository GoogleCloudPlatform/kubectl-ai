#!/usr/bin/env bash

# Delete the namespaces which will also delete all resources in them
kubectl delete namespace ns1 --ignore-not-found
kubectl delete namespace ns2 --ignore-not-found

echo "Cleanup completed" 