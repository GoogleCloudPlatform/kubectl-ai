package tools

import (
	"context"
	"os"
	"os/exec"
	"fmt"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func init() {
	RegisterTool(&OcClient{})
}

type OcClient struct{}

func (t *OcClient) Name() string {
	return "oc"
}

func (t *OcClient) Description() string {
	return `Executes an oc (OpenShift Client) command against the user's OpenShift or Kubernetes cluster. Use this tool for OpenShift-specific operations, user management, cluster operator status, operator status, or when you need to query or modify the state of the user's OpenShift/Kubernetes cluster.

KEYWORDS: openshift, oc, user, users, project, projects, role, roles, get users, check openshift user, manage openshift users, whoami, current user, which user am i, logged in user, show user, display user, identity, login, oc whoami, clusteroperator, cluster operator, cluster status, cluster health, get co, oc get co, check clusteroperator, check cluster status, check cluster health, operator, operators, operator status, list operators, get operators, oc get operators, oc get operator, oc describe operator, csv, clusterserviceversion, oc get csv, operator version, operator install, operator upgrade

IMPORTANT: Interactive commands are not supported in this environment. This includes:
- oc exec with -it flag (use non-interactive exec instead)
- oc edit (use oc get -o yaml, oc patch, or oc apply instead)
- oc port-forward (use alternative methods like NodePort or LoadBalancer)

For interactive operations, please use these non-interactive alternatives:
- Instead of 'oc edit', use 'oc get -o yaml' to view, 'oc patch' for targeted changes, or 'oc apply' to apply full changes
- Instead of 'oc exec -it', use 'oc exec' with a specific command
- Instead of 'oc port-forward', use service types like NodePort or LoadBalancer

Examples:
- To check OpenShift users: oc get users
- To get current user: oc whoami
- To check which OpenShift user you are: oc whoami
- To answer 'which openshift user am i': oc whoami
- To answer 'who am i logged in as': oc whoami
- To list projects: oc get projects
- To get user roles: oc describe user <username>
- To check status of ClusterOperator: oc get co -o wide
- To check cluster health: oc get co
- To check a specific operator: oc get co authentication
- To list all operators: oc get co
- To get operator status: oc get co
- To describe an operator: oc describe co <operator-name>
- To list installed operators (ClusterServiceVersions): oc get csv
- To check operator versions: oc get csv
- To check operator install/upgrade status: oc get csv`
}

func (t *OcClient) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {
					Type: gollm.TypeString,
					Description: `The complete oc command to execute. Prefer to use heredoc syntax for multi-line commands. Please include the oc prefix as well.

IMPORTANT: Do not use interactive commands. Instead:
- Use 'oc get -o yaml', 'oc patch', or 'oc apply' instead of 'oc edit'
- Use 'oc exec' with specific commands instead of 'oc exec -it'
- Use service types like NodePort or LoadBalancer instead of 'oc port-forward'

Examples:
user: what pods are running in the cluster?
assistant: oc get pods

user: what is the status of the pod my-pod?
assistant: oc get pod my-pod -o jsonpath='{.status.phase}'

user: I need to edit the pod configuration
assistant: # Option 1: Using patch for targeted changes
oc patch pod my-pod --patch '{"spec":{"containers":[{"name":"main","image":"new-image"}]}}'

# Option 2: Using get and apply for full changes
oc get pod my-pod -o yaml > pod.yaml
# Edit pod.yaml locally
oc apply -f pod.yaml

user: I need to execute a command in the pod
assistant: oc exec my-pod -- /bin/sh -c "your command here"`,
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

func (t *OcClient) Run(ctx context.Context, args map[string]any) (any, error) {
	kubeconfig := ctx.Value(KubeconfigKey).(string)
	workDir := ctx.Value(WorkDirKey).(string)

	commandVal, ok := args["command"]
	if !ok || commandVal == nil {
		return &ExecResult{Error: "oc command not provided or is nil"}, nil
	}

	command, ok := commandVal.(string)
	if !ok {
		return &ExecResult{Error: "oc command must be a string"}, nil
	}

	return runOcCommand(ctx, command, workDir, kubeconfig)
}

func runOcCommand(ctx context.Context, command, workDir, kubeconfig string) (*ExecResult, error) {
	if isInteractive, err := IsInteractiveOcCommand(command); isInteractive {
		return &ExecResult{Error: err.Error()}, nil
	}

	var cmd *exec.Cmd = exec.CommandContext(ctx, lookupBashBin(), "-c", command)
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

func (t *OcClient) IsInteractive(args map[string]any) (bool, error) {
	commandVal, ok := args["command"]
	if !ok || commandVal == nil {
		return false, nil
	}

	command, ok := commandVal.(string)
	if !ok {
		return false, nil
	}

	return IsInteractiveOcCommand(command)
}

// IsInteractiveOcCommand checks for interactive oc commands
func IsInteractiveOcCommand(command string) (bool, error) {
	words := splitCommandWords(command)
	if len(words) == 0 {
		return false, nil
	}
	base := words[0]
	if base != "oc" {
		return false, nil
	}

	isExec := containsAll(words, []string{"exec", "-it"})
	isPortForward := contains(words, "port-forward")
	isEdit := contains(words, "edit")

	if isExec || isPortForward || isEdit {
		return true, fmt.Errorf("interactive mode not supported for oc, please use non-interactive commands")
	}
	return false, nil
}
func splitCommandWords(command string) []string {
	return exec.Command("sh", "-c", "echo "+command).Args[2:]
}

func contains(words []string, word string) bool {
	for _, w := range words {
		if w == word {
			return true
		}
	}
	return false
}

func containsAll(words []string, targets []string) bool {
	found := make(map[string]bool)
	for _, t := range targets {
		found[t] = false
	}
	for _, w := range words {
		if _, ok := found[w]; ok {
			found[w] = true
		}
	}
	for _, v := range found {
		if !v {
			return false
		}
	}
	return true
}
