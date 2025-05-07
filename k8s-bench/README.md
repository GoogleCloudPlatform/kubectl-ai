## k8s-bench

`k8s-bench` is a benchmark for assessing the performance of LLM models for kubernetes related tasks.


### Usage

```sh
# build the k8s-bench binary
go build

# Run evaluation for scale related tasks
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern scale --output-dir <output_directory> --kubeconfig <path/to/kubeconfig>

# Run evaluation for all available tasks
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern all --output-dir <output_directory> --kubeconfig <path/to/kubeconfig>

# Analyze previous evaluation results and output in markdown format (default)
./k8s-bench analyze --input-dir <output_directory>

# Analyze previous evaluation results and output in JSON format
./k8s-bench analyze --input-dir <output_directory> --output-format json

# Save analysis results to a file
./k8s-bench analyze --input-dir <output_directory> --results-filepath ./results.md
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

The `analyze` subcommand will gather the results from previous runs and display them in a tabular format with emoji indicators for success (✅) and failure (❌).
