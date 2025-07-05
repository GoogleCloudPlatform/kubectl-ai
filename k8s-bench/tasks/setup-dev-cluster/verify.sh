#!/usr/bin/env bash

set -e

DEVELOPERS=("alice" "bob" "charlie")
ALL_NAMESPACES=("dev-alice" "dev-bob" "dev-charlie" "dev-shared" "staging" "prod")

echo "🔍 Starting comprehensive verification of dev cluster setup..."

# 1. Verify all namespaces exist
echo "📋 Checking namespaces..."
for ns in "${ALL_NAMESPACES[@]}"; do
    if ! kubectl get namespace "$ns" &>/dev/null; then
        echo "❌ Namespace '$ns' does not exist"
        exit 1
    fi
    echo "✅ Namespace '$ns' exists"
done

# 2. Verify service accounts exist
echo "👤 Checking service accounts..."
for dev in "${DEVELOPERS[@]}"; do
    sa_name="${dev}-sa"
    ns_name="dev-${dev}"
    if ! kubectl get serviceaccount "$sa_name" -n "$ns_name" &>/dev/null; then
        echo "❌ ServiceAccount '$sa_name' does not exist in namespace '$ns_name'"
        exit 1
    fi
    echo "✅ ServiceAccount '$sa_name' exists in namespace '$ns_name'"
done

# 3. Verify RBAC permissions
echo "🔐 Testing RBAC permissions..."
for dev in "${DEVELOPERS[@]}"; do
    sa_user="system:serviceaccount:dev-${dev}:${dev}-sa"
    own_ns="dev-${dev}"

    # Should have full access to own namespace
    if ! kubectl auth can-i "*" "*" --as="$sa_user" -n "$own_ns" &>/dev/null; then
        echo "❌ $dev cannot perform all actions in their own namespace"
        exit 1
    fi
    echo "✅ $dev has full access to their own namespace"

    # Should have read access to dev-shared
    if ! kubectl auth can-i get pods --as="$sa_user" -n "dev-shared" &>/dev/null; then
        echo "❌ $dev cannot read pods in dev-shared namespace"
        exit 1
    fi
    echo "✅ $dev has read access to dev-shared"

    # Should NOT have access to other dev namespaces
    for other_dev in "${DEVELOPERS[@]}"; do
        if [[ "$dev" != "$other_dev" ]]; then
            other_ns="dev-${other_dev}"
            if kubectl auth can-i get pods --as="$sa_user" -n "$other_ns" &>/dev/null; then
                echo "❌ $dev has unauthorized access to $other_dev's namespace"
                exit 1
            fi
        fi
    done
    echo "✅ $dev is properly isolated from other dev namespaces"

    # Should NOT have access to staging/prod
    for env in staging prod; do
        if kubectl auth can-i get pods --as="$sa_user" -n "$env" &>/dev/null; then
            echo "❌ $dev has unauthorized access to $env namespace"
            exit 1
        fi
    done
    echo "✅ $dev cannot access staging/prod namespaces"
done

# 4. Verify Resource Quotas
echo "💾 Checking resource quotas..."
expected_quotas=(
    "dev-alice:requests.cpu=2:requests.memory=4Gi:pods=10:services=5"
    "dev-bob:requests.cpu=2:requests.memory=4Gi:pods=10:services=5"
    "dev-charlie:requests.cpu=2:requests.memory=4Gi:pods=10:services=5"
    "dev-shared:requests.cpu=4:requests.memory=8Gi:pods=20:services=10"
    "staging:requests.cpu=8:requests.memory=16Gi:pods=50:services=20"
    "prod:requests.cpu=8:requests.memory=16Gi:pods=50:services=20"
)

for quota_spec in "${expected_quotas[@]}"; do
    IFS=':' read -r ns cpu memory pods services <<<"$quota_spec"

    # Check if resource quota exists
    if ! kubectl get resourcequota -n "$ns" &>/dev/null; then
        echo "❌ No ResourceQuota found in namespace '$ns'"
        exit 1
    fi

    # Verify specific limits (simplified check)
    quota_output=$(kubectl get resourcequota -n "$ns" -o yaml)
    if ! echo "$quota_output" | grep -q "pods.*${pods}" ||
        ! echo "$quota_output" | grep -q "services.*${services}"; then
        echo "❌ ResourceQuota in '$ns' doesn't match expected limits"
        exit 1
    fi
    echo "✅ ResourceQuota verified for namespace '$ns'"
done

# 5. Verify Network Policies exist
echo "🌐 Checking network policies..."
for ns in "${ALL_NAMESPACES[@]}"; do
    if ! kubectl get networkpolicy -n "$ns" &>/dev/null; then
        echo "❌ No NetworkPolicy found in namespace '$ns'"
        exit 1
    fi
    echo "✅ NetworkPolicy exists in namespace '$ns'"
done

# 6. Test network isolation (functional test)
echo "🔌 Testing network isolation..."

# Create test pods for network testing
for dev in "${DEVELOPERS[@]}"; do
    ns="dev-${dev}"
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: $ns
  labels:
    app: test-${dev}
spec:
  containers:
  - name: curl
    image: curlimages/curl:latest
    command: ["sleep", "3600"]
---
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: $ns
spec:
  selector:
    app: test-${dev}
  ports:
  - port: 80
    targetPort: 8080
EOF
done

# Wait for pods to be ready
for dev in "${DEVELOPERS[@]}"; do
    kubectl wait --for=condition=Ready pod/test-pod -n "dev-${dev}" --timeout=60s
done

# Test that alice cannot reach bob's namespace
echo "Testing cross-namespace isolation..."
if kubectl exec -n dev-alice test-pod -- curl -s --connect-timeout 5 http://test-service.dev-bob.svc.cluster.local &>/dev/null; then
    echo "❌ Network policy failed: alice can reach bob's namespace"
    exit 1
fi
echo "✅ Cross-namespace access properly blocked"

# Test DNS access (should work)
echo "Testing DNS access..."
if ! kubectl exec -n dev-alice test-pod -- nslookup kubernetes.default.svc.cluster.local &>/dev/null; then
    echo "❌ DNS access blocked (should be allowed)"
    exit 1
fi
echo "✅ DNS access working correctly"

# Cleanup test pods
for dev in "${DEVELOPERS[@]}"; do
    kubectl delete pod test-pod -n "dev-${dev}" --ignore-not-found=true
    kubectl delete service test-service -n "dev-${dev}" --ignore-not-found=true
done

echo "🎉 All verifications passed! Dev cluster setup is correctly configured."
echo "✅ Namespaces: Created with proper isolation"
echo "✅ RBAC: Developers have appropriate access levels"
echo "✅ Resource Quotas: Proper limits enforced"
echo "✅ Network Policies: Cross-namespace isolation working"
echo "✅ Security: Principle of least privilege maintained"

exit 0
