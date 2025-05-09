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
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func init() {
	RegisterTool(&Kubectl{})
}

type Kubectl struct{}

func (t *Kubectl) Name() string {
	return "kubectl"
}

func (t *Kubectl) Description() string {
	return "Executes a kubectl command against the user's Kubernetes cluster. Use this tool only when you need to query or modify the state of the user's Kubernetes cluster."
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
Example:
user: what pods are running in the cluster?
assistant: kubectl get pods

user: what is the status of the pod my-pod?
assistant: kubectl get pod my-pod -o jsonpath='{.status.phase}'
`,
				},
				"modifies_resource": {
					Type: gollm.TypeString,
					Description: `Whether the command modifies a kubernetes resource.
Possible values:
- "yes" if the command modifies a resource
- "no" if the command does not modify a resource
- "unknown" if the command's effect on the resource is unknown
`,
				},
			},
		},
	}
}

func (t *Kubectl) Run(ctx context.Context, args map[string]any) (any, error) {
	kubeconfig := ctx.Value(KubeconfigKey).(string)
	workDir := ctx.Value(WorkDirKey).(string)
	command := args["command"].(string)

	return runKubectlCommand(ctx, command, workDir, kubeconfig)
}

func runKubectlCommand(ctx context.Context, command, workDir, kubeconfig string) (*ExecResult, error) {
	if strings.Contains(command, "kubectl edit") {
		return &ExecResult{Error: "interactive mode not supported for kubectl, please use non-interactive commands"}, nil
	}
	if strings.Contains(command, "kubectl port-forward") {
		return &ExecResult{Error: "port-forwarding is not allowed because assistant is running in an unattended mode, please try some other alternative"}, nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, os.Getenv("COMSPEC"), "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, bashBin, "-c", command)
	}
	cmd.Env = os.Environ()
	cmd.Dir = workDir
	if kubeconfig != "" {
		kubeconfig, err := expandShellVar(kubeconfig)
		if err != nil {
			return nil, err
		}
		cmd.Env = append(cmd.Env, "KUBECONFIG="+kubeconfig)
	}

	return executeCommand(cmd)
}
