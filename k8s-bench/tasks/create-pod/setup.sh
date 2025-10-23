#!/usr/bin/env bash
NAMESPACE="web-server"
kubectl delete namespace $NAMESPACE --ignore-not-found
kubectl create namespace $NAMESPACE