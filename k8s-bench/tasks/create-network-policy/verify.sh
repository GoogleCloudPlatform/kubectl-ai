#!/usr/bin/env bash

# Check if NetworkPolicy exists
if ! kubectl get networkpolicy np -n ns1 &>/dev/null; then
    echo "NetworkPolicy 'np' does not exist in namespace 'ns1'"
    exit 1
fi

echo "✅ NetworkPolicy 'np' exists in namespace 'ns1'"

# Functional test: Verify ingress traffic is not affected (pod in ns2 can reach pod in ns1)
echo "Testing that ingress traffic from ns2 to ns1 is not affected..."
INGRESS_TEST=$(kubectl exec -n ns2 curl-ns2 -- curl -s --connect-timeout 120s http://httpd-ns1.ns1.svc.cluster.local || echo "Failed")
if [[ "$INGRESS_TEST" == "Failed" ]]; then
    echo "Failed to connect from ns2 to ns1 - NetworkPolicy should not restrict incoming traffic"
    exit 1
fi
echo "✅ Ingress traffic from ns2 to ns1 is allowed as expected"

# Functional test: Test connectivity from ns1 to ns2
echo "Testing connectivity from ns1 to ns2..."
CURL_RESULT=$(kubectl exec -n ns1 curl-ns1 -- curl -s --connect-timeout 120s http://httpd-ns2.ns2.svc.cluster.local || echo "Failed")
if [[ "$CURL_RESULT" == "Failed" ]]; then
    echo "Failed to connect from ns1 to ns2 - NetworkPolicy might be too restrictive"
    exit 1
fi
echo "✅ Pods in ns1 can reach pods in ns2 as expected"

# Functional test: Try to connect to something outside ns2
echo "Testing that connections outside ns2 are blocked..."
CURL_RESULT=$(kubectl exec -n ns1 curl-ns1 -- curl -s --connect-timeout 10s https://kubernetes.io || echo "Failed")
if [[ "$CURL_RESULT" != "Failed" ]]; then
    echo "NetworkPolicy should prevent connections to external sites, but connection succeeded"
    exit 1
fi
echo "✅ Pods in ns1 cannot reach external sites as expected"

# More comprehensive DNS resolution test
echo "Testing DNS resolution for internal services..."
DNS_INTERNAL=$(kubectl exec -n ns1 curl-ns1 -- nslookup kubernetes.default.svc.cluster.local || echo "Failed")
if [[ "$DNS_INTERNAL" == "Failed" ]]; then
    echo "DNS resolution for internal services failed - NetworkPolicy might block DNS traffic"
    exit 1
fi

echo "Testing DNS resolution for external domains..."
DNS_EXTERNAL=$(kubectl exec -n ns1 curl-ns1 -- nslookup kubernetes.io || echo "Failed")
if [[ "$DNS_EXTERNAL" == "Failed" ]]; then
    echo "DNS resolution for external domains failed - NetworkPolicy might block DNS traffic"
    exit 1
fi
echo "✅ DNS resolution works as expected"

# All verifications passed
echo "🎉 All verifications passed! NetworkPolicy is correctly configured."
exit 0 