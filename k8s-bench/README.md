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
KUBECONFIG=<path/to/kubeconfig> ./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern scale --output-dir .build/k8sbench

# Pass additional arguments directly to the agent
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --agent-args=--model=gemini-2.5-pro --agent-args=--quiet=true --output-dir .build/k8sbench

# Provide multiple agent arguments in a single flag
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --agent-args="--yolo --prompt" --output-dir .build/k8sbench

# Enforce a two-minute hard timeout for each agent execution
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --agent-timeout=2m --output-dir .build/k8sbench

# Run evaluation sequentially (one task at a time)
./k8s-bench run --agent-bin <path/to/kubectl-ai/binary> --tasks-dir ./tasks --output-dir .build/k8sbench --concurrency 1

# Run evaluation with all available options
./k8s-bench run \
  --agent-bin <path/to/kubectl-ai/binary> \
  --tasks-dir ./tasks \
  --task-pattern fix \
  --agent-args=--model=gemini-2.5-pro-preview-03-25 \
  --agent-args=--quiet=true \
  --concurrency 0 \
  --create-kind-cluster \
  --output-dir .build/k8sbench
```

#### Available flags for `run` subcommand:

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--agent-bin` | Path to kubectl-ai binary | - | Yes |
| `--output-dir` | Directory to write results to | - | Yes |
| `--tasks-dir` | Directory containing evaluation tasks | ./tasks | No |
| `--task-pattern` | Pattern to filter tasks (e.g. 'pod' or 'redis') | - | No |
| `--agent-args` | Additional argument passed directly to the agent (repeat flag to provide multiple) | - | No |
| `--concurrency` | Number of tasks to run concurrently (0 = auto based on number of tasks, 1 = sequential, N = run N tasks at a time) | 0 | No |
| `--create-kind-cluster` | Create a temporary kind cluster for the evaluation run | false | No |
| `--agent-timeout` | Maximum duration to allow each agent invocation before it is terminated | 5m | No |

> **Note:** The agent is invoked with the `KUBECONFIG` environment variable. If unset, it defaults to `~/.kube/config`.

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

### Contributions

We're open to contributions in k8s-bench, check out the [contributions guide.](contributing.md)
