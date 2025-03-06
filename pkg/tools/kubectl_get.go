package tools

import (
	"context"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

// KubectlGet lists kubernetes resources of the given type.
type KubectlGet struct {
	// The type of resource that should be listed. Required.
	Kind string
	// Only show resources with the given name.
	Name string
	// Only show resources in the given namespace.
	Namespace string
	// Show resources in all namespaces.
	AllNamespaces bool
	// Output determines the format of the output.  Valid values: yaml, json, name.
	Output string
}

// Execute runs the tool, when the LLM calls it.
func (x *KubectlGet) Execute(ctx context.Context) (any, error) {
	command := x.buildCommand()

	return runCommand(ctx, command)
}

func (x *KubectlGet) buildCommand() []string {
	command := []string{"kubectl", "get"}
	if x.Namespace != "" {
		command = append(command, "--namespace", x.Namespace)
	} else {
		// TODO: verify all_namespaces was not set to false?
		command = append(command, "--all-namespaces")
	}
	command = append(command, x.Kind)
	if x.Name != "" {
		command = append(command, x.Name)
	}
	if x.Output != "" {
		command = append(command, "-o"+x.Output)
	}
	return command
}

// BuildInfo builds a description of the tool invocation.
func (x *KubectlGet) BuildFunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name: "kubectl_get",
		Description: `kubectl_get lists kubernetes resources of the given type.

If you want to list resources in all namespaces, pass the AllNamespaces parameter as true.

If you want list resources in only one namespace, pass the namespace parameter.

If you want to list resources with a given name, pass the name parameter.
`,
		Parameters: &gollm.Schema{
			Type:     gollm.TypeObject,
			Required: []string{"kind"},
			Properties: map[string]*gollm.Schema{
				"kind": {
					Type:        gollm.TypeString,
					Description: "The type of resource the should be listed.  Required.",
				},
				"name": {
					Type:        gollm.TypeString,
					Description: "Only show resources with the given name.",
				},
				"namespace": {
					Type:        gollm.TypeString,
					Description: "Only show resources in the given namespace.",
				},
				"all_namespaces": {
					Type:        gollm.TypeBoolean,
					Description: "Show resources in all namespaces.",
				},
				"output": {
					Type:        gollm.TypeString,
					Description: "Output determines the format of the output.  Valid values: yaml, json, name.",
				},
			},
		},
	}
}
