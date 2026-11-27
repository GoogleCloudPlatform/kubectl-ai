# Running kubectl-ai safely in production

This guide collects practical tips for using `kubectl-ai` against a production cluster. The
guiding idea is simple: an LLM with cluster credentials is a new kind of actor, so treat it like
one — give it the least privilege it needs, run its tools in a sandbox, keep a human in the
approval loop, and point it at an LLM provider you already trust with your data.

Nothing here is required to try `kubectl-ai`. It is about not regretting it later.

## 1. Give it least privilege

The most important control is not the confirmation prompt — it is what the credentials can do if
the model proposes something wrong. Assume that will eventually happen.

**Use a dedicated namespace and a dedicated context.** Run `kubectl-ai` with a kubeconfig whose
current context points at a single namespace you are willing to have modified:

```bash
# A dedicated context so the agent never inherits your day-to-day admin context
kubectl config set-context kubectl-ai-prod \
  --namespace=my-team \
  --cluster=production \
  --user=kubectl-ai-prod

kubectl-ai --kubeconfig ~/.kube/kubectl-ai-prod.yaml
```

**Prefer read-only by default.** A `ClusterRole`/`Role` granting only `get`, `list`, `watch` lets
the agent diagnose almost anything while changing nothing:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubectl-ai-reader
  namespace: my-team
rules:
  - apiGroups: ["", "apps", "batch", "networking.k8s.io"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
  # Secret values are deliberately NOT granted - see the note below
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubectl-ai-reader-binding
  namespace: my-team
subjects:
  - kind: ServiceAccount
    name: kubectl-ai
    namespace: my-team
roleRef:
  kind: Role
  name: kubectl-ai-reader
  apiGroup: rbac.authorization.k8s.io
```

**Never run it as `cluster-admin`.** If you genuinely need write access, scope it: a `Role` in one
namespace, with `resourceNames` on the specific objects, is a far better backstop than a prompt you
might click through on autopilot.

A useful reference point ships with this repo: the sandbox manifest
(`k8s/sandbox/all-in-one.yaml`) creates the `computer` namespace, a `normal-user` service
account, and a Role named **`reader-all-but-secrets`** that grants `get/list/watch` on pods,
logs, configmaps, events, services, deployments, jobs and similar objects — but explicitly
**not** `secrets`. That is a deliberate, sensible default worth copying: an agent almost never
needs secret *values* to diagnose a workload.

Remember that RBAC applies to whatever identity the agent uses, so a dedicated ServiceAccount (or
a dedicated user in a dedicated kubeconfig) is what makes all of the above enforceable.

## 2. Run tool execution in a sandbox

`kubectl-ai` can execute the commands it suggests. Sandboxing keeps those commands off the machine
running the agent:

```bash
# Run tool execution in Kubernetes (see the manifest note below)
kubectl-ai --sandbox=k8s

# Pin the container image used for sandbox pods
kubectl-ai --sandbox=k8s --sandbox-image=bitnami/kubectl:latest
```

`--sandbox` accepts:

| value | where it runs | notes |
|---|---|---|
| `k8s` | sandbox pods inside your cluster | apply `k8s/sandbox/all-in-one.yaml` first — it creates the `computer` namespace and the `normal-user` ServiceAccount used by `kubectl-ai-sandbox-*` pods |
| `seatbelt` | macOS Seatbelt (sandbox-exec) | macOS only; the binary returns an explicit error on other platforms |

The k8s sandbox is the one to reach for in a server/CI setting: tool commands run in an isolated
pod with the reader RBAC described above, so a bad command is contained to a namespace the agent
can only read from.

See [`docs/gke-deployment.md`](gke-deployment.md) for a complete worked example of deploying
`kubectl-ai` with the sandbox to Google Kubernetes Engine.

## 3. Keep the approval loop, and know when it disappears

By default `kubectl-ai` asks before running commands that **modify** resources. Read-only
commands run without a prompt; anything that changes state produces a confirmation listing the
exact commands:

```
The following commands require your approval to run:
* scale deployment/web --replicas=0

Do you want to proceed ?
```

The choices are **Yes**, **Yes, and don't ask me again**, and **No**. Prefer plain *Yes* during
production work — "don't ask me again" persists for the session and removes the very control you
are relying on.

**Non-interactive mode fails safe.** In `--quiet` (run-once) mode there is nobody to answer the
prompt, so the run **stops and reports an error instead of proceeding**:

```bash
kubectl-ai --quiet "scale the web deployment to zero"
# -> RunOnce mode cannot handle permission requests. The following commands require approval:
#    ...
#    Use --skip-permissions flag to bypass permission checks in RunOnce mode.
```

That is the behaviour you want in CI: a pipeline that accidentally proposes a destructive change
exits non-zero rather than applying it.

**Treat `--skip-permissions` as a production red line.** It exists for throwaway clusters and for
tests, where the whole loop is non-interactive and the blast radius is nil. Combined with
`--quiet --skip-permissions`, an agent will execute modifying commands with no human in the loop at
all. If you ever think you need that in production, the answer is not the flag — it is tighter
RBAC (section 1), so that the modifying command fails on the API server even when the agent tries
it.

Two more loop bounds worth setting:

```bash
# Cap the number of agent iterations so a confused loop cannot run all afternoon
kubectl-ai --max-iterations=10
```

## 4. Choose the LLM provider with your data policy

Prompts and tool output are sent to the model you configure, so pick a provider the same way you
would pick any other processor of production data.

For enterprise use, `vertexai` is the natural choice: it runs under your own Google Cloud project
and IAM, with regional data residency and the data-governance terms you already have in place.

```bash
export GOOGLE_CLOUD_PROJECT="my-gcp-project"
export GOOGLE_CLOUD_LOCATION="us-central1"        # pin the region for data residency

kubectl-ai --llm-provider=vertexai --model=gemini-2.0-flash
```

Authentication uses Application Default Credentials, so run under a dedicated service account in
production rather than a personal login:

```bash
gcloud auth application-default login        # developer workstation
# in production, prefer a workload identity / attached service account
```

Practical data-handling notes:

- **Pin the region.** `GOOGLE_CLOUD_LOCATION` decides where inference happens; pick the region your
  data-residency policy allows instead of taking a default.
- **Pin the model.** `--model` keeps behaviour reproducible across runs and deployments; a
  floating model alias can change under you.
- **Prefer private endpoints where available.** For `openai`-compatible deployments, `--llm-provider`
  accepts a self-hosted base URL, and local providers (`ollama`, `llama.cpp`) keep every byte on
  your own hardware, at the cost of model quality.
- **Assume everything the tools print is sent to the model.** That is why the reader role in
  section 1 omits `secrets`: an agent that can read Secret values can also ship them to an LLM.
- `--skip-verify-ssl` disables TLS verification for the LLM provider. It exists for lab setups;
  leave it off in production.

## 5. Keep an audit trail

When an agent changes a cluster, "what did it run, and why?" should be answerable afterwards.

```bash
# Persist conversations so a session can be reviewed and resumed
kubectl-ai --session-backend=filesystem --new-session

# List and revisit earlier sessions
kubectl-ai --list-sessions
kubectl-ai --resume-session=latest

# Write a trace file for offline inspection
kubectl-ai --trace-path=kubectl-ai-trace.json

# Show the raw tool output in the terminal UI
kubectl-ai --show-tool-output
```

Sessions default to the in-memory backend; asking for session flags upgrades them to
`filesystem` automatically. Keep `--trace-path` output with your normal change records — unlike
the cluster, it shows the *reasoning* behind a change, which is exactly what is missing when you
review a diff later.

Also note that the temporary working directory used during a run is removed with
`--remove-workdir`, which is a good default on shared hosts.

## 6. Hardening checklist

Starting from a stock install, a production-shaped invocation looks like this:

```bash
export GOOGLE_CLOUD_PROJECT="my-gcp-project"
export GOOGLE_CLOUD_LOCATION="us-central1"

kubectl-ai \
  --llm-provider=vertexai \
  --model=gemini-2.0-flash \
  --kubeconfig ~/.kube/kubectl-ai-prod.yaml \
  --sandbox=k8s \
  --max-iterations=10 \
  --session-backend=filesystem \
  --trace-path=/var/log/kubectl-ai/trace.json
```

And the boxes to tick before you point it at anything that matters:

- [ ] RBAC for the agent's identity is read-only, or scoped to a single namespace/object set.
- [ ] `secrets` are **not** readable by that identity.
- [ ] The kubeconfig used is dedicated to the agent, not your personal admin context.
- [ ] `--sandbox=k8s` is on (with `k8s/sandbox/all-in-one.yaml` applied) for any untrusted command.
- [ ] `--skip-permissions` is **absent**; interactive approvals are answered deliberately.
- [ ] The LLM provider is one your data policy allows, with the region and model pinned.
- [ ] Sessions and/or `--trace-path` are retained for review.
- [ ] `--max-iterations` is set to something you would be comfortable paying for.

## Scope

This guide covers the safety controls that `kubectl-ai` exposes today. It does not cover cluster
hardening in general (network policies, admission control, pod security standards) — those are
worth doing for their own sake, and they also bound what an agent like this can do. Where the two
overlap, the cluster-side control is the one that still holds when the model has a bad day.
