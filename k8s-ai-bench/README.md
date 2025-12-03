## k8s-ai-bench

`k8s-ai-bench` is a benchmark for assessing the performance of LLM models for kubernetes related tasks.


### Usage

```sh
# build the k8s-ai-bench binary
go build
```

#### Run Subcommand

The `run` subcommand executes the benchmark evaluations.

```sh
# Basic usage with mandatory output directory
./k8s-ai-bench run --agent-bin <path/to/kubectl-ai/binary> --output-dir .build/k8s-ai-bench

# Run evaluation for scale related tasks
./k8s-ai-bench run --agent-bin <path/to/kubectl-ai/binary> --task-pattern scale --kubeconfig <path/to/kubeconfig> --output-dir .build/k8s-ai-bench

# Run evaluation for a specific LLM provider and model with tool use shim enabled
./k8s-ai-bench run --llm-provider=grok --models=grok-3-beta --agent-bin kubectl-ai --task-pattern=fix-probes --enable-tool-use-shim=true --output-dir .build/k8s-ai-bench

# Run evaluation sequentially (one task at a time)
./k8s-ai-bench run --agent-bin <path/to/kubectl-ai/binary> --tasks-dir ./tasks --output-dir .build/k8s-ai-bench --concurrency 1

# Run evaluation with all available options
./k8s-ai-bench run \
  --agent-bin <path/to/kubectl-ai/binary> \
  --kubeconfig ~/.kube/config \
  --tasks-dir ./tasks \
  --task-pattern fix \
  --llm-provider gemini \
  --models gemini-2.5-pro-preview-03-25,gemini-1.5-pro-latest \
  --enable-tool-use-shim true \
  --quiet \
  --concurrency 0 \
  --output-dir .build/k8s-ai-bench
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
| `--enable-tool-use-shim` | Enable tool use shim | false | No |
| `--quiet` | Quiet mode (non-interactive mode) | true | No |
| `--concurrency` | Number of tasks to run concurrently (0 = auto based on number of tasks, 1 = sequential, N = run N tasks at a time) | 0 | No |
| `--mcp-client` | Enable MCP client in kubectl-ai | false | No |
| `--cluster-creation-policy` | Cluster creation policy: AlwaysCreate, CreateIfNotExist, DoNotCreate | CreateIfNotExist | No |

#### Analyze Subcommand

The `analyze` subcommand processes results from previous runs:

```sh
# Analyze previous evaluation results and output in markdown format (default)
./k8s-ai-bench analyze --input-dir .build/k8s-ai-bench

# Analyze previous evaluation results and output in JSON format
./k8s-ai-bench analyze --input-dir .build/k8s-ai-bench --output-format json

# Save analysis results to a file
./k8s-ai-bench analyze --input-dir .build/k8s-ai-bench --results-filepath ./results.md

# Analyze with all available options
./k8s-ai-bench analyze \
  --input-dir .build/k8s-ai-bench \
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
| `--show-failures` | Show failure details in markdown output | false | No |

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

### Running evaluations with dev scripts

For a streamlined experience, you can use the provided dev scripts to run the evaluation suite.

#### Run evaluations (preferred method)

The `run-eval-loop.sh` script runs the evaluations a specified number of times, creating a separate output directory for each iteration. This is useful for testing the consistency of a model's performance. This will also automatically create markdown and json analysis files for each run. 

```sh
# Run the evaluation loop 5 times for tasks matching the "create" pattern
./dev/ci/periodics/run-eval-loop.sh --iterations 5 --task-pattern "create"
```

Available flags for `run-eval-loop.sh`:

| Flag | Description | Default |
|------|-------------|---------|
| `-i, --iterations` | Number of times to run the loop | 1 |
| `-p, --provider` | The LLM provider to use | gemini |
| `-m, --model` | The specific model to test | gemini-2.5-pro |
| `-a, --api-base` | The API base URL | http://localhost:8000/v1 |
| `-c, --concurrency` | The number of eval tasks to run in parallel | 5 |
| `-t, --task-pattern` | The regex pattern for tasks to run | |
| `-k, --cluster-creation-policy` | kind cluster creation policy | CreateIfNotExists |

#### Run evaluations

The `run-evals.sh` script builds the necessary binaries and runs the evaluations a single time. You can pass arguments to the `k8s-ai-bench run` command via the `TEST_ARGS` environment variable.

```sh
# Run all tasks
./dev/ci/periodics/run-evals.sh

# Run a specific task
TEST_ARGS="--task-pattern=fix-probes" ./dev/ci/periodics/run-evals.sh

# Run with a different provider and model
TEST_ARGS="--llm-provider=openai --models=openai/gpt-oss-20b" ./dev/ci/periodics/run-evals.sh
```

#### Analyze results

The `analyze-evals.sh` script analyzes the results from the previous run.

```sh
# Analyze the last run
./dev/ci/periodics/analyze-evals.sh

# Show failures in the analysis
./dev/ci/periodics/analyze-evals.sh --show-failures
```

The results will be saved to `.build/k8s-ai-bench.md` and `.build/k8s-ai-bench.json`.

### Visualization

The `site` directory contains a static website for visualizing the benchmark results.

To view the visualization locally:

1.  Ensure you have a `combined_results.jsonl` file in the `site` directory. You can generate this by running the `analyze` command with the `jsonl` output format:
    ```sh
    ./k8s-ai-bench analyze --input-dir .build/k8s-ai-bench --output-format jsonl --results-filepath site/combined_results.jsonl
    ```
2.  Serve the site locally (browsers block `file://` access to local files for security reasons):
    ```sh
    cd site
    python3 -m http.server
    ```
3.  Open [http://localhost:8000](http://localhost:8000) in your browser.

### Contributions

We're open to contributions in k8s-ai-bench, check out the [contributions guide.](contributing.md)
