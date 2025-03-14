## k8s-bench

`k8s-bench` is a benchmark for assessing the performance of LLM models for kubernetes related tasks.


### Usage

```sh
# build the k8s-bench binary
go build

# Run evaluation for scale related tasks
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern scale --kubeconfig <path/to/kubeconfig>

# Analyze previous evaluation results and output in markdown format (default)
./k8s-bench analyze --input-dir .build/k8sbench

# Analyze previous evaluation results and output in JSON format
./k8s-bench analyze --input-dir .build/k8sbench --output-format json

# Save analysis results to a file
./k8s-bench analyze --input-dir .build/k8sbench --results-filepath ./results.md
```

Running the benchmark with the `run` subcommand will produce results as below:

```sh
Evaluation Results:
==================

Task: scale-deployment
  Provider: gemini
    gemini-2.0-flash-thinking-exp-01-21: true

Task: scale-down-deployment
  Provider: gemini
    gemini-2.0-flash-thinking-exp-01-21: true
```

The `analyze` subcommand will gather the results from previous runs and display them in a tabular format with emoji indicators for success (✅) and failure (❌). Results are grouped by strategy:

```markdown
# K8s-bench Evaluation Results

## Strategy: chat-based

| Task | Provider | Model | Result | Error |
|------|----------|-------|--------|-------|
| scale-deployment | gemini | gemini-2.0-flash | ✅ success | |
| scale-down-deployment | gemini | gemini-2.0-flash | ✅ success | |

**chat-based Summary**

- Total: 20
- Success: 15 (75%)
- Fail: 5 (25%)

## Strategy: react

| Task | Provider | Model | Result | Error |
|------|----------|-------|--------|-------|
| scale-deployment | gemini | gemini-2.0-flash | ✅ success | |
| scale-down-deployment | gemini | gemini-2.0-flash | ✅ success | |

**react Summary**

- Total: 20
- Success: 13 (65%)
- Fail: 7 (35%)

## Model Performance Summary

| Model | chat-based Success | chat-based Fail | react Success | react Fail |
|-------|--------------|-----------|------------|-----------|
| gemini-2.0-flash | 5 | 2 | 5 | 2 |
| gemini-2.0-flash-thinking-exp-01-21 | 5 | 2 | 4 | 3 |
| gemma-3-27b-it | 5 | 1 | 4 | 2 |
| **Total** | 15 | 5 | 13 | 7 |

## Overall Summary

- Total: 40
- Success: 28 (70%)
- Fail: 12 (30%)