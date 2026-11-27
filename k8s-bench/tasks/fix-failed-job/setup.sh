#!/usr/bin/env bash
kubectl delete namespace job-test --ignore-not-found
kubectl create namespace job-test

# Create a job that will fail due to a typo in the command
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: data-processor
  namespace: job-test
spec:
  backoffLimit: 6
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: processor
        image: busybox:1.36
        command: ["/bin/sh", "-c"]
        args: ["ech 'Processing data...' && sleep 1"]  # Typo: 'ech' instead of 'echo'
EOF

# Wait for job to fail at least once
for i in {1..30}; do
    if kubectl get job data-processor -n job-test -o jsonpath='{.status.failed}' | grep -q "[1-9]"; then
        exit 0
    fi
    sleep 1
done
