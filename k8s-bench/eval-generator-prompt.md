# ROLE AND GOAL

You are an expert-level Site Reliability Engineer (SRE) with extensive Kubernetes experience and knowledge of best practices.

Your goal is to generate a complete and self-contained evaluation task designed to test an AI agent's ability to perform Kubernetes-related tasks. You will produce a set of files that can be used to set up the scenario, present the problem to the AI, verify the solution, and clean up the environment.

---

# TASK CRITERIA

Each generated evaluation must adhere to the following principles:

1.  **Realistic:** The scenario must reflect a real-world problem an engineer would face. The prompt for the AI agent should be conversational and natural. Avoid unnecessary hints to help pass, and avoid providing info that lets the agent know it’s being tested.
2.  **Self-Contained:** All Kubernetes resources MUST be created in a dedicated, unique namespace. This namespace should be defined as a variable (`NAMESPACE`) at the top of each shell script for consistency and to prevent conflicts.
3.  **Verifiable:** The `verify.sh` script must contain specific, automatable checks (e.g., using `kubectl get ... -o jsonpath='...'` or `grep`). The script **must** `exit 0` on success and `exit 1` on failure.
4.  **Robust:** The `setup.sh` script should be idempotent (safe to run multiple times). The `cleanup.sh` script must reliably delete the dedicated namespace and all its resources.

---

# REQUIRED OUTPUT FORMAT

You must generate the complete contents for four separate files: `task.yaml`, `setup.sh`, `verify.sh`, and `cleanup.sh`. You can also include an `artifacts/` directory that includes any other necessary scripts or resources needed for the eval. All these should be in an appropriately-named directory under k8s-bench/tasks. Use the exact markdown formatting below, including the file names in the headers and the language-specific code fences for the content. Do not include any other explanatory text outside of the code blocks.



**task.yaml**
```yaml
script:
  - prompt: {A natural language prompt for the AI agent}
setup: "setup.sh"
verifier: "verify.sh"
cleanup: "cleanup.sh"
difficulty: {easy|medium|hard}
```



**setup.sh**
```bash
#!/bin/bash
set -e
NAMESPACE="eval-task-$(date +%s)"
# Add setup commands here
```



**verify.sh**
```bash
#!/bin/bash
set -e
NAMESPACE={The exact same namespace as setup.sh}
# Add verification logic here. Exit 0 on success, 1 on failure.
```



**cleanup.sh**
```bash
#!/bin/bash
set -e
NAMESPACE={The exact same namespace as setup.sh}
kubectl delete namespace $NAMESPACE --wait=false
```



GOLDEN EXAMPLE

This is an example of a perfect output for the task: "Fix a Pod that is stuck in a CrashLoopBackOff state due to a bad command."
**task.yaml**
```yaml
script:
  - prompt: "Hey, I just deployed my 'finance-app' in the `finance-ns` namespace, but the pod seems to be stuck in a crash loop. Can you please figure out what's wrong and fix it so the pod runs successfully?
setup: "setup.sh"
verifier: "verify.sh"
cleanup: "cleanup.sh"
difficulty: "easy"
```


**setup.sh**
```bash
#!/bin/bash
set -e
# Use a unique namespace for this evaluation
NAMESPACE="finance-ns"

# Create the namespace
kubectl create namespace $NAMESPACE

# Create the broken deployment
kubectl apply -n $NAMESPACE -f ./artifacts/finance-app.yaml
```


**verify.sh**
```bash
#!/bin/bash
set -e
NAMESPACE=”finance-ns”
DEPLOYMENT_NAME=”finance-app”

# Wait for the deployment to become available
echo “Waiting for deployment $DEPLOYMENT_NAME to be available…”
kubectl rollout status deployment/$DEPLOYMENT_NAME -n $NAMESPACE --timeout=30s

# Wait for the pod to be in a running state
echo “Waiting for pods to be in a ‘Running’ state…”
kubectl wait –for=condition=Ready pod -l app=$DEPLOYMENT_NAME -n $NAMESPACE --timeout=30s

echo "Verification successful!"
exit 0
```


**cleanup.sh**
```bash
#!/bin/bash
set -e
NAMESPACE=”finance-ns”

# Delete the namespace
kubectl delete namespace $NAMESPACE --wait=false
```

**artifacts/finance-app.yaml**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: finance-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: finance-app
  template:
    metadata:
      labels:
        app: finance-app
    spec:
      containers:
      - name: main
        image: busybox:1.36
        # This command is invalid and will cause a crash
        command: ["/bin/sh", "-c", "echo 'starting...' && sleep 5 && exit 1"]
```


**THE TASK**

Now, using the role, criteria, format, and golden example above as your guide, generate a complete evaluation for the following user-provided task.\
TASK: "{INSERT_EVALUATION_TOPIC_HERE}"


