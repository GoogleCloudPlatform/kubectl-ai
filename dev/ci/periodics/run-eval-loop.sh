#!/bin/bash

set -eou pipefail

# Example usage: ./run-eval-loop.sh --iterations 5 --provider gemini --model gemini-2.5-pro --api-base http://localhost:8000/v1 --concurrency 5 --task-pattern "create" -k AlwaysCreate

# Number of times to run the loop (default: 3)
ITERATIONS=3
# The LLM provider to use (default: gemini)
PROVIDER="gemini"
# The specific model to test (default: "gemini-2.5-pro")
MODEL="gemini-2.5-pro"
# The API base URL (default: "http://localhost:8000/v1")
API_BASE="http://localhost:8000/v1"
# The number of eval tasks to run in parallel (default: 5)
CONCURRENCY=1
# The regex pattern for tasks to run
TASK_PATTERN=""
# kind cluster creation policy (default: "CreateIfNotExists")
CLUSTER_CREATION_POLICY="CreateIfNotExists"

while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -i|--iterations)
      ITERATIONS="$2"
      shift 2
      ;;
    -p|--provider)
      PROVIDER="$2"
      shift 2
      ;;
    -m|--model)
      MODEL="$2"
      shift 2
      ;;
    -a|--api-base)
      API_BASE="$2"
      shift 2
      ;;
    -c|--concurrency)
      CONCURRENCY="$2"
      shift 2
      ;;
    -t|--task-pattern)
      TASK_PATTERN="$2"
      shift 2
      ;;
    -k|--cluster-creation-policy)
      CLUSTER_CREATION_POLICY="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)    # unknown option
      echo "Unknown option: $1"
      usage
      ;;
  esac
done

# Check for required commands
for cmd in git go make; do
  if ! command -v $cmd &> /dev/null; then
    echo "Error: Required command '$cmd' is not installed. Aborting." >&2
    exit 1
  fi
done

# Build kubectl-ai and k8s-bench binaries
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd ${REPO_ROOT}

BINDIR="${REPO_ROOT}/.build/bin"
mkdir -p "${BINDIR}"

cd "${REPO_ROOT}/cmd"
go build -o "${BINDIR}/kubectl-ai" .

cd "${REPO_ROOT}/k8s-bench"
go build -o "${BINDIR}/k8s-bench" .
K8S_BENCH_BIN="${BINDIR}/k8s-bench"

# Go back to REPO_ROOT to start running evals
cd "${REPO_ROOT}"

# Start evaluation loop
echo "Starting evaluation loop..."
echo "Runs:      $ITERATIONS"
echo "Provider:  $PROVIDER"
echo "Model:     $MODEL"
echo "API Base:  $API_BASE"
echo "Concurrency: $CONCURRENCY"
echo "Task Pattern: ${TASK_PATTERN:-"All Tasks"}"

# Loop from 1 to the specified number of iterations
for i in $(seq 1 $ITERATIONS)
do
  # Create the unique directory name for this run
  OUTPUT_DIR="${REPO_ROOT}/.build/k8s-bench-${MODEL}-${i}"
  
  echo "**********"
  echo "loop_evals: Running iteration $i of $ITERATIONS..."
  echo "writing results to $OUTPUT_DIR"
  echo "**********"

  # Construct the arguments for the make command
  TEST_ARGS="--output-dir=${OUTPUT_DIR} --cluster-creation-policy=${CLUSTER_CREATION_POLICY} --concurrency ${CONCURRENCY} "
  TEST_ARGS+="--agent-args=--enable-tool-use-shim=false "
  TEST_ARGS+="--agent-args=--llm-provider=${PROVIDER} "
  TEST_ARGS+="--agent-args=--model=${MODEL} "
  TEST_ARGS+="--agent-args=--quiet "

  # Add task pattern if it was supplied
  if [ -n "$TASK_PATTERN" ]; then
    TEST_ARGS+="--task-pattern=${TASK_PATTERN} "
    echo "Applying task pattern: ${TASK_PATTERN}"
  fi

  # Execute the make command and capture the evaluation time line
  run_time_line=$( \
    OPENAI_API_KEY="not needed" \
    OPENAI_API_BASE="$API_BASE" \
    TEST_ARGS="$TEST_ARGS" \
    make run-evals | tee /dev/tty | grep '^Total evaluation time:' \
  )

  # Check for errors in the make command
  if [ ${PIPESTATUS[0]} -ne 0 ]; then
    echo "Error on iteration $i during 'make run-evals'. Aborting loop."
    exit 1
  fi

  echo "---"
  echo "Analyzing results for iteration $i..."
  
  # Paths for analysis files
  MARKDOWN_FILE="${OUTPUT_DIR}/k8s-bench.md"
  JSON_FILE="${OUTPUT_DIR}/k8s-bench.json"

  # Run for markdown format
  "${K8S_BENCH_BIN}" analyze --input-dir="${OUTPUT_DIR}" --results-filepath="${MARKDOWN_FILE}" --output-format=markdown --show-failures
  if [ $? -ne 0 ]; then
    echo "Error on iteration $i during Markdown analysis. Aborting loop."
    exit 1
  fi

  # Run for json format
  "${K8S_BENCH_BIN}" analyze --input-dir="${OUTPUT_DIR}" --results-filepath="${JSON_FILE}" --output-format=json --show-failures
  if [ $? -ne 0 ]; then
    echo "Error on iteration $i during JSON analysis. Aborting loop."
    exit 1
  fi

  # Extract the time value and append it to the markdown file
  if [ -n "$run_time_line" ]; then
    time_value=$(echo $run_time_line | awk '{print $4}')
    
    # Append the time to the markdown file with some formatting
    echo "" >> "${MARKDOWN_FILE}"
    echo "---" >> "${MARKDOWN_FILE}"
    echo "**Total evaluation time:** ${time_value}" >> "${MARKDOWN_FILE}"
    
    echo "Appended evaluation time to ${MARKDOWN_FILE}"
  else
    echo "Warning: Could not find evaluation time for iteration $i."
  fi

  echo "Finished iteration $i."
done

echo "All $ITERATIONS runs completed successfully!"