# Custom Tools for kubectl-ai

The `kubectl-ai` assistant can be extended with custom tools to interact with various command-line interfaces (CLIs) beyond `kubectl`. This allows the AI to perform a wider range of tasks related to infrastructure management, CI/CD, and more.

This document outlines the available tools, their locations, and how to use them.

## Available Tools

The following tools are configured by default.

### Argo CD (`argocd`)
A declarative, GitOps continuous delivery tool for Kubernetes. Use it to manage application deployments from Git repositories.

### GitHub CLI (`gh`)
The official GitHub command-line tool. Use it to interact with GitHub repositories, pull requests, issues, actions, and more, directly from the terminal.

### Google Cloud CLI (`gcloud`)
The primary CLI for Google Cloud. Use it to manage Google Cloud resources, including Google Kubernetes Engine (GKE) clusters, virtual machines, and networking.

### Kustomize (`kustomize`)
A tool to customize Kubernetes resource configurations. Use it to render and apply declarative configurations from a directory containing a `kustomization.yaml` file.

## Tool Configuration Locations

The YAML configuration files for these tools can be found in the following locations:

-   **GitHub Repository:** The source files are located in the `pkg/tools/samples/` directory.
-   **Docker Image:** The tools are pre-loaded into the official Docker image at `/etc/kubectl-ai/tools/`.

## Using Custom Tools

To enable the custom tools, you must point `kubectl-ai` to the directory containing the tool configuration YAML files using the `--custom-tools-config` flag.

### Running from a Local Binary

When running the `kubectl-ai` binary directly, provide the path to your local tools directory.

```sh
./kubectl-ai --custom-tools-config=/path/to/kubectl-ai/pkg/tools/samples "your prompt here"
```

### Running with Docker

When using the Docker image, you can either use the tools baked into the image or mount your own custom directory.

#### Using Built-in Tools

The official Docker image includes the default tool configurations. You can enable them by pointing to the internal path.

```sh
docker run --rm -it your-kubectl-ai-image:latest \
  --custom-tools-config=/etc/kubectl-ai/tools \
  "list all pull requests on GitHub"
```

#### Using a Local Tools Directory

To use a custom set of tools from your local machine, mount the directory into the container and point the flag to the mounted path. This is useful for developing and testing new tools.

```sh
docker run --rm -it \
  -v /path/to/your/local/tools:/my-custom-tools \
  your-kubectl-ai-image:latest \
  --custom-tools-config=/my-custom-tools \
  "your prompt here"
```
