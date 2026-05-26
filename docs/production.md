# Production use guidance

kubectl-ai can execute Kubernetes and shell commands on behalf of the user. When
using it with production clusters, treat it like any other operator workstation:
limit the credentials it can reach, keep command approval enabled, and preserve
enough logs to reconstruct what happened.

This guide focuses on operational safeguards. It does not replace your
organization's Kubernetes, identity, or data-handling policies.

## Start with a narrow kubeconfig

Use a kubeconfig and Kubernetes identity that match the task you want kubectl-ai
to perform. Avoid running production sessions with a personal cluster-admin
context.

Good defaults:

- use a dedicated service account or user identity for kubectl-ai sessions
- bind only the namespaces, resources, and verbs needed for the workflow
- keep separate kubeconfig files for dev, staging, and production clusters
- verify the active context before starting a session

```sh
kubectl config current-context
kubectl auth can-i list pods --namespace production
kubectl auth can-i delete deployments --namespace production
```

If kubectl-ai only needs to investigate cluster state, start with read-only
permissions. Add write permissions only after you have a clear operational
reason.

## Keep command approval enabled

Leave `skipPermissions` set to `false` unless you are running in a tightly
controlled automation environment.

```yaml
skipPermissions: false
```

With command approval enabled, review commands before they run. Pay special
attention to commands that:

- mutate resources, such as `apply`, `patch`, `delete`, `scale`, or `rollout`
- read secrets, config maps, or logs with sensitive content
- pipe output into shell scripts
- use custom tools or MCP tools outside the built-in kubectl flow

For non-interactive use, prefer prompts that ask kubectl-ai to explain the
planned commands first. Run the mutating step only after reviewing the plan.

## Use sandboxed command execution

When possible, run commands in a sandbox instead of directly on the local
machine. kubectl-ai supports sandbox execution with `--sandbox k8s` and
`--sandbox seatbelt`.

```sh
kubectl-ai --sandbox k8s "inspect the rollout status for the payments service"
```

For GKE deployments, see the [GKE deployment guide](gke-deployment.md). It
describes the sandbox resources and how to verify that commands run inside
sandbox pods.

Sandboxing is not a substitute for Kubernetes RBAC. The sandbox should still use
a limited kubeconfig and a namespace with only the permissions needed for the
session.

## Choose a model provider deliberately

Pick an LLM provider that matches your organization's data-handling and access
requirements. For Google Cloud environments, Vertex AI is often easier to align
with existing project, IAM, logging, and data governance controls than a
personal API key.

Example configuration:

```yaml
llmProvider: vertexai
model: gemini-2.5-flash-preview-04-17
kubeconfig: ~/.kube/kubectl-ai-prod-readonly
skipPermissions: false
sandbox: k8s
```

Before using production data, confirm what prompts, command outputs, logs, and
tool results may be sent to the selected provider.

## Restrict custom tools

Custom tools extend what kubectl-ai can execute. In production, load only the
tool configs required for the current workflow.

```sh
kubectl-ai \
  --custom-tools-config ~/.config/kubectl-ai/production-tools \
  "summarize unhealthy workloads in the payments namespace"
```

Keep production tool configs small and reviewable. Prefer commands that inspect
state over commands that perform broad mutations. See
[Custom Tools for kubectl-ai](tools.md) for the tool configuration format.

## Keep an audit trail

For production sessions, record enough context to explain later what happened:

- the kubeconfig or service account used
- the cluster context and namespace
- the user prompt
- approved commands
- command output needed for incident or change records
- follow-up changes made outside kubectl-ai

If you enable session persistence, store the session files in a location covered
by your retention and access-control policies.

## Suggested rollout path

1. Run kubectl-ai against a local or disposable cluster.
2. Test read-only production prompts with a read-only kubeconfig.
3. Add sandboxed execution and verify commands run in the sandbox.
4. Add narrowly scoped write permissions for a single namespace or workflow.
5. Review and document the approval process before using it during incidents.

This staged approach keeps the first production use small enough to audit while
still letting teams build operational confidence.
