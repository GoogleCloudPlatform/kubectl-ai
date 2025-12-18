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

BINDIR="${REPO_ROOT}/.build/bin"
mkdir -p "${BINDIR}"

K8S_AI_BENCH_SRC="${REPO_ROOT}/.build/k8s-ai-bench-src"
rm -rf "${K8S_AI_BENCH_SRC}"
git clone https://github.com/gke-labs/k8s-ai-bench "${K8S_AI_BENCH_SRC}"
cd "${K8S_AI_BENCH_SRC}"
GOWORK=off go build -o "${BINDIR}/k8s-ai-bench" .

cd "${REPO_ROOT}"

# Pass --show-failures flag to the analyze command if it's set
ANALYZE_ARGS=""
if [[ "$*" == *"--show-failures"* ]]; then
    ANALYZE_ARGS="--show-failures"
fi

"${BINDIR}/k8s-ai-bench" analyze --input-dir "${OUTPUT_DIR}" ${TEST_ARGS:-} -results-filepath ${REPO_ROOT}/.build/k8s-ai-bench.md --output-format markdown ${ANALYZE_ARGS}
"${BINDIR}/k8s-ai-bench" analyze --input-dir "${OUTPUT_DIR}" ${TEST_ARGS:-} -results-filepath ${REPO_ROOT}/.build/k8s-ai-bench.json --output-format json
