# k8s-bench Eval Tasks

This directory contains evaluation tasks for testing kubectl-ai capabilities on realistic Kubernetes scenarios.

## About

The main evaluation framework lives in the [k8s-ai-bench](https://github.com/gke-labs/k8s-ai-bench) repository. Tasks defined here follow the same structure and can be contributed to the upstream repository.

## Task Structure

Each task is a directory containing:

```
tasks/
└── <task-name>/
    ├── task.yaml      # Task definition (prompt, difficulty)
    ├── setup.sh       # Creates the problematic state
    ├── verify.sh      # Verifies the fix was successful
    ├── cleanup.sh     # Cleans up resources
    └── artifacts/     # Optional: K8s manifests
```

### task.yaml Format

```yaml
script:
- prompt: "Your prompt to kubectl-ai"
setup: "setup.sh"
verifier: "verify.sh"
cleanup: "cleanup.sh"
difficulty: "easy|medium|hard"
```

## Contributing New Tasks

1. **Identify a realistic scenario** that users commonly encounter
2. **Create a task directory** following the structure above
3. **Submit to k8s-ai-bench** via pull request to [gke-labs/k8s-ai-bench](https://github.com/gke-labs/k8s-ai-bench)

## Current Tasks

| Task | Description | Difficulty |
|------|-------------|------------|
| fix-failed-job | Debug and fix a failing Kubernetes Job | medium |

## Running Evals

See the [k8s-ai-bench documentation](https://github.com/gke-labs/k8s-ai-bench) for instructions on running evaluations.
