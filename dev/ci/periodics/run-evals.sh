#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

set -x

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd ${REPO_ROOT}

if [[ -z "${OUTPUT_DIR:-}" ]]; then
    OUTPUT_DIR="${REPO_ROOT}/.build/k8s-ai-bench"
    mkdir -p "${OUTPUT_DIR}"
fi
echo "Writing results to ${OUTPUT_DIR}"

BINDIR="${REPO_ROOT}/.build/bin"
mkdir -p "${BINDIR}"

curl -sSL https://raw.githubusercontent.com/GoogleCloudPlatform/kubectl-ai/main/install.sh | bash

K8S_AI_BENCH_SRC="${REPO_ROOT}/.build/k8s-ai-bench-src"
rm -rf "${K8S_AI_BENCH_SRC}"
git clone https://github.com/gke-labs/k8s-ai-bench "${K8S_AI_BENCH_SRC}"
cd "${K8S_AI_BENCH_SRC}"
GOWORK=off go build -o "${BINDIR}/k8s-ai-bench" .

"${BINDIR}/k8s-ai-bench" run --agent-bin kubectl-ai --kubeconfig "${KUBECONFIG:-~/.kube/config}" --output-dir "${OUTPUT_DIR}" ${TEST_ARGS:-}
