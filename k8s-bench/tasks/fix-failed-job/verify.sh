#!/usr/bin/env bash

# Configuration
JOB_NAME="data-processor"
NAMESPACE="job-test"
TIMEOUT="120s"

echo "Starting verification for fix-failed-job..."

# Wait for job to complete successfully
echo "Waiting for Job '$JOB_NAME' to complete successfully..."
if ! kubectl wait --for=condition=Complete job/$JOB_NAME -n $NAMESPACE --timeout=$TIMEOUT; then
    echo "Job '$JOB_NAME' did not complete successfully within $TIMEOUT."
    echo "---"
    echo "Job status:"
    kubectl describe job $JOB_NAME -n $NAMESPACE
    echo "---"
    echo "Recent pod logs:"
    kubectl logs -l job-name=$JOB_NAME -n $NAMESPACE --tail=20 || echo "No logs available"
    exit 1
fi

echo "Job '$JOB_NAME' completed successfully. Verification passed."
exit 0
