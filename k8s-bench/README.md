## k8s-bench

`k8s-bench` is a benchmark for assessing the performance of LLM models for kubernetes related tasks.


### Usage

```sh
# build the k8s-bench binary
go build
```

#### Run Subcommand

The `run` subcommand executes the benchmark evaluations.

```sh
# Basic usage with mandatory output directory
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --output-dir .build/k8sbench

# Run evaluation for scale related tasks
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern scale --kubeconfig <path/to/kubeconfig> --output-dir .build/k8sbench

# Run evaluation for a specific LLM provider and model with tool use shim disabled
./k8s-bench run --llm-provider=grok --models=grok-3-beta --agent-bin ../kubectl-ai --task-pattern=fix-probes --enable-tool-use-shim=false --output-dir .build/k8sbench

# Run evaluation sequentially (one task at a time)
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --tasks-dir ./tasks --output-dir .build/k8sbench --concurrency 1

# Run evaluation with all available options
./k8s-bench run \
  --agent-bin <path/to/kubectl-ai/binary> \
  --kubeconfig ~/.kube/config \
  --tasks-dir ./tasks \
  --task-pattern fix \
  --llm-provider gemini \
  --models gemini-2.5-pro-preview-03-25,gemini-1.5-pro-latest \
  --enable-tool-use-shim true \
  --quiet true \
  --concurrency 0 \
  --output-dir .build/k8sbench
```

#### Available flags for `run` subcommand:

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--agent-bin` | Path to kubectl-ai binary | - | Yes |
| `--output-dir` | Directory to write results to | - | Yes |
| `--tasks-dir` | Directory containing evaluation tasks | ./tasks | No |
| `--kubeconfig` | Path to kubeconfig file | ~/.kube/config | No |
| `--task-pattern` | Pattern to filter tasks (e.g. 'pod' or 'redis') | - | No |
| `--llm-provider` | Specific LLM provider to evaluate (e.g. 'gemini' or 'ollama') | gemini | No |
| `--models` | Comma-separated list of models to evaluate | gemini-2.5-pro-preview-03-25 | No |
| `--enable-tool-use-shim` | Enable tool use shim | true | No |
| `--quiet` | Quiet mode (non-interactive mode) | true | No |
| `--concurrency` | Number of tasks to run concurrently (0 = auto based on number of tasks, 1 = sequential, N = run N tasks at a time) | 0 | No |

#### Analyze Subcommand

The `analyze` subcommand processes results from previous runs:

```sh
# Analyze previous evaluation results and output in markdown format (default)
./k8s-bench analyze --input-dir .build/k8sbench

# Analyze previous evaluation results and output in JSON format
./k8s-bench analyze --input-dir .build/k8sbench --output-format json

# Save analysis results to a file
./k8s-bench analyze --input-dir .build/k8sbench --results-filepath ./results.md

# Analyze with all available options
./k8s-bench analyze \
  --input-dir .build/k8sbench \
  --output-format markdown \
  --ignore-tool-use-shim true \
  --results-filepath ./detailed-analysis.md
```

#### Available flags for `analyze` subcommand:

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--input-dir` | Directory containing evaluation results | - | Yes |
| `--output-format` | Output format (markdown or json) | markdown | No |
| `--ignore-tool-use-shim` | Ignore tool use shim in result grouping | true | No |
| `--results-filepath` | Optional file path to write results to | - | No |

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
