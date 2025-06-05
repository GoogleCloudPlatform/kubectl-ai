module github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench

go 1.24.0

toolchain go1.24.1

replace github.com/GoogleCloudPlatform/kubectl-ai => ./..

require (
	k8s.io/klog/v2 v2.130.1
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
)
