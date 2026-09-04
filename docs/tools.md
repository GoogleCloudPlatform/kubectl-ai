# Custom Tools for kubectl-ai

`kubectl-ai` leverages LLMs to suggest and execute Kubernetes operations using a set of powerful tools. It comes with built-in tools like `kubectl` and `bash`.

The `kubectl-ai` assistant can be extended with custom tools to interact with various command-line interfaces (CLIs) beyond `kubectl`. This allows the AI to perform a wider range of tasks related to infrastructure management, CI/CD, and more.

This document outlines how you can add custom tools by detailing the steps and providing samples.

This document also outlines the available tools, their locations, and how to use them.

## Adding Custom Tools

Custom tools can be added by following these two steps:

- describing or templating the tool through YAML file
- enabling the tool in the configuration file by pointing the **--custom-tools-config** to this file / directory

## Describing the Tool in YAML file

A custom tool can be described by providing the following four pieces of information:

- **name**: name of the tool
- **description**: "A clear description that helps the LLM understand when to use this tool."
- **command** : "your_command" # For example: 'gcloud' or 'gcloud container clusters'
- **command_desc**: "Detailed information for the LLM, including command syntax and usage examples."

Samples are provided in the [`docs/tool-samples`](./tool-samples) directory. Below is a sample for the `kustomize` tool:

```yaml
- name: kustomize
  description: "A tool to customize Kubernetes resource configurations. Use it to render and apply declarative configurations from a directory containing a kustomization.yaml file."
  command: "kustomize"
  command_desc: |
    The kustomize command-line interface.
    
    Core subcommands and usage patterns:
    - `kustomize build <kustomization_dir>`: Prints the customized resources to standard output. This is useful for inspecting the final configuration before applying it.
    - `kustomize build <kustomization_dir> | kubectl apply -f -`: A common pattern to apply the output directly to the cluster.
    
    Note: `kubectl apply -k <dir>` is a shorthand for the pipe command above and is often preferred.
```

## Enabling the Custom Tool

To enable the custom tools, you must point `kubectl-ai` to the directory containing the tool configuration YAML files using the `--custom-tools-config` flag. `kubectl-ai` can pick up a single YAML file (e.g., `tools.yaml`) containing all the tool descriptions or multiple individual YAML files when pointed to a directory containing them. This example uses multiple YAML files located in a single directory.

In case, you don't want to use a tool (that is provided in samples), just move the file out of the directory into some other location & restart `kubectl-ai`.

### Running from a Local Binary

When running the `kubectl-ai` binary directly, provide the path to your local tools directory.

```sh
./kubectl-ai --custom-tools-config=<path-to-tools-directory> "your prompt here"
```

### Running with Docker Image

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

## How to Create a Custom Tool Config from Scratch

The following walkthrough shows how to build a config for the GitHub CLI (`gh`). The same pattern applies to any CLI-backed tool.

1. **Pick a short tool name** the LLM will call (for example, `gh`).
2. **Write a one-line description** that tells the LLM when to use the tool.
3. **Set `command`** to the binary name on your `PATH`.
4. **Document subcommands in `command_desc`** with concrete examples the LLM can follow.

```yaml
- name: gh
  description: "The official GitHub command-line tool. Use it to interact with GitHub repositories, pull requests, issues, actions, and more, directly from the terminal."
  command: "gh"
  command_desc: |
    The gh command-line interface for GitHub.

    Core subcommands and usage patterns:
    - `gh auth login`: Authenticate with a GitHub host. This is required before most other commands.
    - `gh pr list`: List pull requests in the current repository.
    - `gh pr view <number>`: View a specific pull request.
    - `gh issue list`: List issues in the current repository.

    Use `gh --help` or `gh <subcommand> --help` for full syntax and available flags.
```

5. **Save the file** (for example, `~/.config/kubectl-ai/tools/gh.yaml`).
6. **Point kubectl-ai at the file or directory** with `--custom-tools-config` or `toolConfigPaths` in `~/.config/kubectl-ai/config.yaml`.
7. **Verify the tool is loaded** by running `tools` in an interactive session.

Tips:

- Focus `command_desc` on the subcommands and flags that matter for your workflows.
- Keep one tool per YAML file when sharing examples; a single `tools.yaml` can also list multiple tools.
- Remove or move unused sample files out of your tools directory if you do not want kubectl-ai to load them.

## Sample Custom Tools

The following curated example configs ship with kubectl-ai and are copied into the Docker image at `/etc/kubectl-ai/tools`.

| Tool | Description | YAML File |
| :--- | :---------- | :-------- |
| Argo CD (`argocd`) | A declarative, GitOps continuous delivery tool for Kubernetes. | [argocd.yaml](./tool-samples/argocd.yaml) |
| GitHub CLI (`gh`) | The official command-line tool to interact with GitHub. | [gh.yaml](./tool-samples/gh.yaml) |
| Google Cloud CLI (`gcloud`) | The primary CLI for managing Google Cloud resources. | [gcloud.yaml](./tool-samples/gcloud.yaml) |
| Helm (`helm`) | The Helm package manager for Kubernetes. | [helm.yaml](./tool-samples/helm.yaml) |
| Istio CLI (`istioctl`) | The Istio command-line tool for service mesh operations. | [istioctl.yaml](./tool-samples/istioctl.yaml) |
| Kustomize (`kustomize`) | A tool to customize Kubernetes resource configurations. | [kustomize.yaml](./tool-samples/kustomize.yaml) |
