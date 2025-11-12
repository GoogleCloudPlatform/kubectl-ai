# Deploying k8-kate to Google Kubernetes Engine

## Prerequisites

- A GKE cluster (Standard or Autopilot).
- `gcloud` CLI authenticated with `gcloud auth login`.
- Local Docker environment (or Cloud Build) capable of building and pushing container images.
- [`kubectl` configured](https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl) to talk to your target cluster.
- A Gemini API key with access to the model you plan to demo.

## 1. Set your Google Cloud context

```bash
export PROJECT_ID="my-gcp-project"
export REGION="us-central1"
export CLUSTER_NAME="kubectl-ai-demo"

gcloud config set project "${PROJECT_ID}"
gcloud container clusters get-credentials "${CLUSTER_NAME}" --region "${REGION}"
```

These commands configure both `gcloud` and `kubectl` to operate on the cluster that will host `kubectl-ai`.

## 2. Build and push the kubectl-ai image

Pick an Artifact Registry repository or another container registry that your cluster can pull from. The snippet below creates an Artifact Registry repo (if needed), builds the image locally, and pushes it.

```bash
# Create an Artifact Registry (skip if you already have one)
gcloud artifacts repositories create kubectl-ai \
  --location="${REGION}" \
  --repository-format=DOCKER \
  --description="kubectl-ai demo images"

# Configure Docker to authenticate to Artifact Registry
gcloud auth configure-docker "${REGION}"-docker.pkg.dev

# Build and push the container
IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/kubectl-ai/kubectl-ai:latest"
docker build -t "${IMAGE}" -f images/kubectl-ai/Dockerfile .
docker push "${IMAGE}"
```
## 3. Prepare cluster namespaces and RBAC

Create the namespaces and RBAC that the hosted agent requires:

```bash
# Namespace that hosts the kubectl-ai Deployment
kubectl create namespace kubectl-ai

# Sandbox namespace + RBAC (creates `computer` namespace, service account, and reader roles)
kubectl apply -f k8s/sandbox/all-in-one.yaml
```

The sandbox manifest provisions the `computer` namespace and the `normal-user` service account used for sandbox pods. The Deployment manifest expects the `kubectl-ai` namespace to exist ahead of time.

## 4. Configure the deployment manifest

Copy `k8s/kubectl-ai-gke.yaml` to a working file and edit the following sections:

1. **Container image** – replace the `REPLACE_WITH_YOUR_IMAGE` 
2. **Gemini API key** – change `REPLACE_WITH_YOUR_GEMINI_API_KEY` to the key you obtained from Google AI Studio

Review the RBAC objects in the manifest and adjust them if your security posture requires tighter permissions.

## 5. Deploy kubectl-ai

Apply the updated manifest to your cluster:

```bash
kubectl apply -f kubectl-ai-gke.yaml
```

Kubernetes creates the Deployment, ServiceAccount, RBAC bindings, and Service for the hosted agent. You can watch the rollout with:

```bash
kubectl get pods -n kubectl-ai
kubectl describe pod -n kubectl-ai -l app=kubectl-ai | grep -i image
```

## 6. Access the hosted web UI

Port-forward the Service locally to interact with the hosted UI:

```bash
kubectl port-forward svc/kubectl-ai -n kubectl-ai 8080:80
```

Then open [http://localhost:8080](http://localhost:8080) in your browser. Each browser session can create, rename, and delete conversations, and messages stream in real time via Server-Sent Events.

If you prefer to expose the UI via an external Load Balancer, replace the Service type in the manifest with `LoadBalancer` and configure the appropriate firewall rules.

## 7. Verify sandboxed tool execution

When the UI creates a conversation, the agent launches a sandbox pod in the `computer` namespace. You can confirm sandbox activity with:

```bash
kubectl get pods -n computer
```

Pods named `kubectl-ai-sandbox-*` indicate that commands are running inside isolated helper containers. Asking the agent to execute commands such as `uname -a` should produce output that matches the sandbox image (for example `bitnami/kubectl`).

## 8. Cleanup

Remove the deployment and sandbox resources when you are done:

```bash
kubectl delete -f kubectl-ai-gke.yaml
kubectl delete namespace kubectl-ai
kubectl delete -f k8s/sandbox/all-in-one.yaml
```

If you no longer need the Artifact Registry repository or pushed image, delete them using `gcloud artifacts repositories delete` and `gcloud artifacts docker images delete`.
