// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
)

func init() {
	RegisterTool(&Kubectl{})
}

type Kubectl struct{}

func (t *Kubectl) Name() string {
	return "kubectl"
}

func (t *Kubectl) Description() string {
	return `Executes a kubectl command against the user's Kubernetes cluster. Use this tool only when you need to query or modify the state of the user's Kubernetes cluster.

IMPORTANT: Interactive commands are not supported in this environment. This includes:
- kubectl exec with -it flag (use non-interactive exec instead)
- kubectl edit (use kubectl get -o yaml, kubectl patch, or kubectl apply instead)
- kubectl port-forward (use alternative methods like NodePort or LoadBalancer)

For interactive operations, please use these non-interactive alternatives:
- Instead of 'kubectl edit', use 'kubectl get -o yaml' to view, 'kubectl patch' for targeted changes, or 'kubectl apply' to apply full changes
- Instead of 'kubectl exec -it', use 'kubectl exec' with a specific command
- Instead of 'kubectl port-forward', use service types like NodePort or LoadBalancer`
}

func (t *Kubectl) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {
					Type: gollm.TypeString,
					Description: `The complete kubectl command to execute. Prefer to use heredoc syntax for multi-line commands. Please include the kubectl prefix as well.

IMPORTANT: Do not use interactive commands. Instead:
- Use 'kubectl get -o yaml', 'kubectl patch', or 'kubectl apply' instead of 'kubectl edit'
- Use 'kubectl exec' with specific commands instead of 'kubectl exec -it'
- Use service types like NodePort or LoadBalancer instead of 'kubectl port-forward'

Examples:
user: what pods are running in the cluster?
assistant: kubectl get pods

user: what is the status of the pod my-pod?
assistant: kubectl get pod my-pod -o jsonpath='{.status.phase}'

user: I need to edit the pod configuration
assistant: # Option 1: Using patch for targeted changes
kubectl patch pod my-pod --patch '{"spec":{"containers":[{"name":"main","image":"new-image"}]}}'

# Option 2: Using get and apply for full changes
kubectl get pod my-pod -o yaml > pod.yaml
# Edit pod.yaml locally
kubectl apply -f pod.yaml

user: I need to execute a command in the pod
assistant: kubectl exec my-pod -- /bin/sh -c "your command here"`,
				},
				"modifies_resource": {
					Type: gollm.TypeString,
					Description: `Whether the command modifies a kubernetes resource.
Possible values:
- "yes" if the command modifies a resource
- "no" if the command does not modify a resource
- "unknown" if the command's effect on the resource is unknown`},
			},
		},
	}
}

func (t *Kubectl) Run(ctx context.Context, args map[string]any) (any, error) {
	kubeconfig := ctx.Value(KubeconfigKey).(string)
	workDir := ctx.Value(WorkDirKey).(string)

	// Add nil check for command
	commandVal, ok := args["command"]
	if !ok || commandVal == nil {
		return &sandbox.ExecResult{Command: "", Error: "kubectl command not provided or is nil"}, nil
	}

	command, ok := commandVal.(string)
	if !ok {
		return &sandbox.ExecResult{Command: command, Error: "kubectl command must be a string"}, nil
	}

	// Check for interactive commands before proceeding
	if err := validateKubectlCommand(command); err != nil {
		return &sandbox.ExecResult{Command: command, Error: err.Error()}, nil
	}

	// Get executor from context or default to local
	var executor sandbox.Executor
	if v := ctx.Value(ExecutorKey); v != nil {
		executor = v.(sandbox.Executor)
	} else {
		executor = sandbox.NewLocalExecutor()
	}

	// Prepare environment
	env := os.Environ()
	if kubeconfig != "" {
		kubeconfig, err := ExpandShellVar(kubeconfig)
		if err != nil {
			return nil, err
		}
		env = append(env, "KUBECONFIG="+kubeconfig)
	}

	return ExecuteWithStreamingHandling(ctx, executor, command, workDir, env, DetectKubectlStreaming)
}

// DetectKubectlStreaming checks if a kubectl command is a streaming command
func DetectKubectlStreaming(command string) (bool, string) {
	isWatch := strings.Contains(command, " get ") && strings.Contains(command, " -w")
	isLogs := strings.Contains(command, " logs ") && strings.Contains(command, " -f")
	isAttach := strings.Contains(command, " attach ")

	if isWatch {
		return true, "watch"
	}
	if isLogs {
		return true, "logs"
	}
	if isAttach {
		return true, "attach"
	}
	return false, ""
}

func (t *Kubectl) IsInteractive(args map[string]any) (bool, error) {
	commandVal, ok := args["command"]
	if !ok || commandVal == nil {
		return false, nil
	}

	command, ok := commandVal.(string)
	if !ok {
		return false, nil
	}

	return IsInteractiveCommand(command)
}

// CheckModifiesResource determines if the command modifies kubernetes resources
// This is used for permission checks before command execution
// Returns "yes", "no", or "unknown"
func (t *Kubectl) CheckModifiesResource(args map[string]any) string {
	command, ok := args["command"].(string)
	if !ok {
		return "unknown"
	}

	return kubectlModifiesResource(command)
}

func validateKubectlCommand(command string) error {
	if strings.Contains(command, "kubectl edit") {
		return fmt.Errorf("interactive mode not supported for kubectl, please use non-interactive commands")
	}
	if strings.Contains(command, "kubectl port-forward") {
		return fmt.Errorf("port-forwarding is not allowed because assistant is running in an unattended mode, please try some other alternative")
	}
	return nil
}
