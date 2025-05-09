#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

set -x

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd ${REPO_ROOT}

if [[ -z "${OUTPUT_DIR:-}" ]]; then
    OUTPUT_DIR="${REPO_ROOT}/.build/k8sbench"
    mkdir -p "${OUTPUT_DIR}"
fi
echo "Writing results to ${OUTPUT_DIR}"

BINDIR="${REPO_ROOT}/.build/bin"
mkdir -p "${BINDIR}"

cd "${REPO_ROOT}/k8s-bench"
go build -o "${BINDIR}/k8s-bench" .

cd "${REPO_ROOT}"
"${BINDIR}/k8s-bench" analyze --input-dir "${OUTPUT_DIR}" ${TEST_ARGS:-} -results-filepath k8s-bench.md --output-format markdown
"${BINDIR}/k8s-bench" analyze --input-dir "${OUTPUT_DIR}" ${TEST_ARGS:-} -results-filepath k8s-bench.json --output-format json
