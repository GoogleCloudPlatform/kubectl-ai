#!/usr/bin/env bash

kubectl delete namespace multi-container-test --ignore-not-found
kubectl create namespace multi-container-test