#!/usr/bin/env bash
set -euo pipefail

# --- Configuration ---
readonly DEVELOPERS=("alice" "bob" "charlie")
readonly DEV_NAMESPACES=("dev-alice" "dev-bob" "dev-charlie")
readonly ALL_NAMESPACES=("${DEV_NAMESPACES[@]}" "dev-shared" "staging" "prod")
readonly TEST_LABEL="app=verification-test"

# --- Cleanup Function ---
cleanup() {
  echo "Cleaning up test resources..."
  for ns in "${ALL_NAMESPACES[@]}"; do
    kubectl delete namespace "$ns" --ignore-not-found=true --grace-period=0 --force &
  done
  wait # Wait for all background delete jobs to finish
  echo "Cleanup complete."
}
trap cleanup EXIT

# --- Verification Functions ---

# 1. Verify all namespaces exist in a single API call
verify_namespaces() {
  echo "Checking namespaces..."
  local existing_ns
  existing_ns=$(kubectl get namespace -o jsonpath='{.items[*].metadata.name}')
  for ns in "${ALL_NAMESPACES[@]}"; do
    if ! [[ "$existing_ns" =~ $ns ]]; then
      echo "Namespace '$ns' does not exist"
      exit 1
    fi
  done
  echo "All namespaces exist."
}

# 2. Verify service accounts exist
verify_service_accounts() {
  echo "Checking service accounts..."
  for dev in "${DEVELOPERS[@]}"; do
    local sa_name="${dev}-sa"
    local ns_name="dev-${dev}"
    if ! kubectl get serviceaccount "$sa_name" -n "$ns_name" &>/dev/null; then
      echo "ServiceAccount '$sa_name' does not exist in namespace '$ns_name'"
      exit 1
    fi
  done
  echo "All developer ServiceAccounts exist."
}

# 3. Verify RBAC permissions
verify_rbac() {
  echo "Testing RBAC permissions..."
  for dev in "${DEVELOPERS[@]}"; do
    local user_name="${dev}"
    local sa_user="system:serviceaccount:dev-${dev}:${dev}-sa"
    local own_ns="dev-${dev}"
    
    # --- Test *User* Permissions ---
    # Users should have full access to their own namespaces
    if ! kubectl auth can-i create pods --as="$user_name" -n "$own_ns" --quiet; then
      echo "FAIL: $user_name (User) cannot create pods in their own namespace '$own_ns'"
      exit 1
    fi

    # Users should have read access to dev-shared
    if ! kubectl auth can-i get pods --as="$user_name" -n "dev-shared" --quiet; then
      echo "FAIL: $user_name (User) cannot read pods in 'dev-shared' namespace"
      exit 1
    fi

    # --- Test *Service Account* Permissions ---
    # Service account should have full access to its own namespace
    if ! kubectl auth can-i create pods --as="$sa_user" -n "$own_ns" --quiet; then
      echo "FAIL: $sa_user (SA) cannot create pods in their own namespace '$own_ns'"
      exit 1
    fi
    
    # Service account should have read access to dev-shared
    if ! kubectl auth can-i get pods --as="$sa_user" -n "dev-shared" --quiet; then
      echo "FAIL: $sa_user (SA) cannot read pods in 'dev-shared' namespace"
      exit 1
    fi
    
    # --- Test Isolation for *BOTH* identities ---
    for other_ns in "${ALL_NAMESPACES[@]}"; do
      # Skip namespaces where access is expected
      if [[ "$other_ns" == "$own_ns" || "$other_ns" == "dev-shared" ]]; then
        continue
      fi

      # --- User Isolation Check ---
      if kubectl auth can-i get pods --as="$user_name" -n "$other_ns" --quiet; then
        echo "FAIL: $user_name (User) has unauthorized read access to '$other_ns' namespace"
        exit 1
      fi

      # --- Service Account Isolation Check ---
      if kubectl auth can-i get pods --as="$sa_user" -n "$other_ns" --quiet; then
        echo "FAIL: $sa_user (SA) has unauthorized read access to '$other_ns' namespace"
        exit 1
      fi
    done
    
  done
  echo "RBAC permissions are correctly configured."
}

# 4. Verify Resource Quotas using precise jsonpath
verify_quotas() {
  echo "Checking resource quotas..."
  declare -A expected_quotas=(
      ["dev-alice"]="pods=10:services=5"
      ["dev-bob"]="pods=10:services=5"
      ["dev-charlie"]="pods=10:services=5"
      ["dev-shared"]="pods=20:services=10"
      ["staging"]="pods=50:services=20"
      ["prod"]="pods=50:services=20"
  )

  for ns in "${!expected_quotas[@]}"; do
    # First, find the name of the ResourceQuota object in the namespace.
    local quota_name
    quota_name=$(kubectl get resourcequota -n "$ns" -o jsonpath='{.items[0].metadata.name}')

    if [[ -z "$quota_name" ]]; then
      echo "No ResourceQuota object found in namespace '$ns'"
      exit 1
    fi

    # Parse expected values from the array
    local expected_values="${expected_quotas[$ns]}"
    local expected_pods=$(echo "$expected_values" | cut -d: -f1 | cut -d= -f2)
    local expected_services=$(echo "$expected_values" | cut -d: -f2 | cut -d= -f2)

    # Get both actual values in a single API call, separated by a space
    local actual_values
    actual_values=$(kubectl get resourcequota "$quota_name" -n "$ns" -o=jsonpath='{.spec.hard.pods}{" "}{.spec.hard.services}')
    
    # Read the space-separated output into variables
    local actual_pods actual_services
    read -r actual_pods actual_services <<< "$actual_values"

    # Check if either value does not match
    if [[ "$actual_pods" != "$expected_pods" || "$actual_services" != "$expected_services" ]]; then
      echo "ResourceQuota mismatch in namespace '$ns'."
      echo "  - Expected: pods=${expected_pods}, services=${expected_services}"
      echo "  - Found:    pods=${actual_pods}, services=${actual_services}"
      exit 1
    fi
  done
  echo "Resource quotas are correctly configured."
}

# 5. Verify Network Policies exist (without assuming a name)
verify_network_policies() {
  echo "Checking for existence of Network Policies..."
  for ns in "${ALL_NAMESPACES[@]}"; do
    # Get a count of NetworkPolicy objects in the namespace
    local policy_count
    policy_count=$(kubectl get networkpolicy -n "$ns" -o name | wc -l)
    
    if [[ "$policy_count" -eq 0 ]]; then
      echo "No NetworkPolicy objects found in namespace '$ns'. A default deny policy is likely missing."
      exit 1
    fi
  done
  echo "At least one NetworkPolicy exists in all namespaces."
}

# 6. Test network isolation with a functional test
test_network_isolation() {
  echo "Testing network isolation..."
  local manifest_yaml=""

  # Generate a single YAML manifest for all test resources
  for dev in "${DEVELOPERS[@]}"; do
    local ns="dev-${dev}"
    # The selector needs key: value format
    local selector_key="app"
    local selector_value="verification-test"

    manifest_yaml+=$(cat <<EOF
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-${dev}
  namespace: $ns
  labels:
    ${selector_key}: ${selector_value}
spec:
  containers:
  - name: curl
    image: curlimages/curl:latest
    command: ["sleep", "3600"]
    resources:
        limits:
            cpu: "100m"
            memory: "128Mi"
        requests:
            cpu: "100m"
            memory: "128Mi"
---
apiVersion: v1
kind: Service
metadata:
  name: test-service-${dev}
  namespace: $ns
  labels:
    ${selector_key}: ${selector_value}
spec:
  selector:
    ${selector_key}: ${selector_value} # Use same labels for selector
  ports:
  - port: 80
EOF
)
  done

  echo "  - Creating test pods and services..."
  echo "$manifest_yaml" | kubectl apply -f -

  echo "  - Waiting for test pods to be ready..."
  for dev in "${DEVELOPERS[@]}"; do
    kubectl wait --for=condition=Ready pod/test-pod-${dev} -n "dev-${dev}" --timeout=60s
  done
  
  # Test that alice cannot reach bob's service
  echo "  - Testing cross-namespace isolation (alice -> bob)..."
  if kubectl exec -n dev-alice test-pod-alice -- curl -s --max-time 3 http://test-service-bob.dev-bob.svc.cluster.local &>/dev/null; then
    echo "Network policy FAILED: dev-alice can reach dev-bob's service"
    exit 1
  fi
  echo "  - Cross-namespace access is properly blocked."

  # Test that DNS is working
  echo "  - Testing DNS access..."
  if ! kubectl exec -n dev-alice test-pod-alice -- nslookup -timeout=3 kubernetes.default.svc.cluster.local &>/dev/null; then
    echo "DNS resolution FAILED (it should be allowed by network policies)"
    exit 1
  fi
  echo "  - DNS access is working correctly."
  echo "Network policies are functioning correctly."
}


# --- Main Execution ---
main() {
  echo "Starting comprehensive verification of dev cluster setup..."
  verify_namespaces
  verify_service_accounts
  verify_rbac
  verify_quotas
  verify_network_policies
  test_network_isolation
  echo "All verifications passed! Cluster setup is correctly configured."
}

main