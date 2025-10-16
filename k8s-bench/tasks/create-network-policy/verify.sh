#!/usr/bin/env bash

set -euo pipefail

echo "Starting verification for create-network-policy"
POLICY_NAME="np"
NAMESPACE="ns1"

# Check if NetworkPolicy exists
if ! kubectl get networkpolicy $POLICY_NAME -n $NAMESPACE -o name &>/dev/null; then
    echo "Failed: NetworkPolicy '$POLICY_NAME' does not exist in namespace '$NAMESPACE'"
    exit 1
fi

VERIFY_DIR=$(dirname -- "$0")
YAML_FILE="${VERIFY_DIR}/artifacts/desired-policy.yaml"

# This JQ filter normalizes the egress spec completely:
# 1. map(...): Iterates over each rule in the 'egress' array.
# 2. if .ports ...: If it finds a 'ports' rule, it sorts by 'port' and 'protocol' to make the order consistent.
# 3. | sort_by(.): Sorts the top-level 'egress' array itself, so the 'ports' rule and 'to' rule can be in any order.
# The 'jq -S' command handles sorting object keys (like 'port', 'protocol').
JQ_FILTER='.spec.egress | map(if .ports then .ports |= sort_by(.port, .protocol) else . end) | sort_by(.)'

# 1. Get the LIVE egress array, sort it completely
LIVE_EGRESS_SPEC=$(kubectl get networkpolicy $POLICY_NAME -n $NAMESPACE -o json | jq -S "$JQ_FILTER")
if [ -z "$LIVE_EGRESS_SPEC" ]; then
    echo "Failed: Could not retrieve and normalize LIVE egress spec."
    exit 1
fi

# 2. Get the DESIRED egress array from a dry-run, sort it completely
DESIRED_EGRESS_SPEC=$(kubectl apply -f $YAML_FILE --dry-run=server -o json | jq -S "$JQ_FILTER")
if [ -z "$DESIRED_EGRESS_SPEC" ]; then
    echo "Failed: Could not perform and normalize server-side dry-run."
    exit 1
fi

# 3. Compare the two fully-normalized JSON strings
if ! diff -q <(echo "$LIVE_EGRESS_SPEC") <(echo "$DESIRED_EGRESS_SPEC") >/dev/null 2>&1; then
    echo "Failed: NetworkPolicy egress specs don't match (after full normalization):"
    # Pretty-print the diff for a readable failure message
    diff --color=always <(echo "$LIVE_EGRESS_SPEC" | jq) <(echo "$DESIRED_EGRESS_SPEC" | jq)
    exit 1
fi

echo "All verifications passed! NetworkPolicy egress spec is correctly configured."
exit 0