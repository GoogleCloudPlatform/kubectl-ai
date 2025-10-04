# Running kubectl-ai in a Docker Container

## 1. Build the Docker Image

First, clone the `kubectl-ai` repository and build the Docker image from the
source code.

```bash
git clone https://github.com/GoogleCloudPlatform/kubectl-ai.git
cd kubectl-ai
docker build -t kubectl-ai:latest -f images/kubectl-ai/Dockerfile .
```

## 2. Running against a GKE cluster

To access a GKE cluster, `kubectl-ai` needs two configurations from your local
machine: **Google Cloud credentials** and a **Kubernetes config file**.

### Create Google Cloud Credentials

First, create Application Default Credentials
[(ADC)](https://cloud.google.com/docs/authentication/application-default-credentials).
`kubectl` uses these credentials to authenticate with your GKE cluster.

```bash
gcloud auth application-default login
```

This command saves your credentials into the `~/.config/gcloud` directory.

### Configure `kubectl`

Next, generate the `kubeconfig` file. This file tells `kubectl` which cluster
to connect to and to use your ADC credentials for authentication.

```bash
gcloud container clusters get-credentials <cluster-name> --location <location>
```

This updates the configuration file at `~/.kube/config`.

## 3. Running the Container

Finally, mount both configuration directories into the `kubectl-ai` container
when you run it. This example shows how to run `kubectl-ai` with a web
interface, mounting all necessary credentials and providing a Gemini API key.

```bash
export GEMINI_API_KEY="your_api_key_here"
docker run --rm -it -p 8080:8080 \
  -v ~/.kube:/root/.kube \
  -v ~/.config/gcloud:/root/.config/gcloud \
  -e GEMINI_API_KEY \
  kubectl-ai:latest \
  --ui-listen-address 0.0.0.0:8080 \
  --ui-type web
```

Alternatively with the default terminal ui:

```bash
export GEMINI_API_KEY="your_api_key_here"
docker run --rm -it \
  -v ~/.kube:/root/.kube \
  -v ~/.config/gcloud:/root/.config/gcloud \
  -e GEMINI_API_KEY \
  kubectl-ai:latest
```
