package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
)

// envCapturingExecutor captures the env passed to Execute.
type envCapturingExecutor struct {
	capturedEnv []string
}

func (e *envCapturingExecutor) Execute(ctx context.Context, command string, env []string, workDir string) (*sandbox.ExecResult, error) {
	e.capturedEnv = env
	return &sandbox.ExecResult{Command: command, Stdout: "ok"}, nil
}

func (e *envCapturingExecutor) Close(ctx context.Context) error { return nil }

func TestKubectlRunSetsColumnsEnv(t *testing.T) {
	executor := &envCapturingExecutor{}
	tool := &Kubectl{executor: executor}

	ctx := context.WithValue(context.Background(), KubeconfigKey, "")
	ctx = context.WithValue(ctx, WorkDirKey, "/tmp")

	_, err := tool.Run(ctx, map[string]any{"command": "kubectl get nodes -o wide"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, env := range executor.capturedEnv {
		if env == "COLUMNS=256" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("COLUMNS=256 not found in env passed to executor; env = %v", executor.capturedEnv)
	}
}

func TestKubectlRunSetsColumnsWithKubeconfig(t *testing.T) {
	executor := &envCapturingExecutor{}
	tool := &Kubectl{executor: executor}

	ctx := context.WithValue(context.Background(), KubeconfigKey, "/home/user/.kube/config")
	ctx = context.WithValue(ctx, WorkDirKey, "/tmp")

	_, err := tool.Run(ctx, map[string]any{"command": "kubectl get pods"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasColumns := false
	hasKubeconfig := false
	for _, env := range executor.capturedEnv {
		if env == "COLUMNS=256" {
			hasColumns = true
		}
		if strings.HasPrefix(env, "KUBECONFIG=") {
			hasKubeconfig = true
		}
	}
	if !hasColumns {
		t.Error("COLUMNS=256 not found in env")
	}
	if !hasKubeconfig {
		t.Error("KUBECONFIG not found in env")
	}
}
