package tools

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

// runCommand execs the command and returns the output.
func runCommand(ctx context.Context, command []string) (map[string]any, error) {
	log := klog.FromContext(ctx)

	stdin := ""
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(stdin)

	log.V(2).Info("running command", "command", strings.Join(cmd.Args, " "), "stdin", stdin)

	if err := cmd.Run(); err != nil {
		log.Error(err, "running command", "stdout", stdout.String(), "stderr", stderr.String())
		ret := map[string]any{
			"exit_code": cmd.ProcessState.ExitCode(),
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
		}
		return ret, nil
	}

	ret := map[string]any{
		"content": stdout.String(),
	}
	return ret, nil
}

// formatCommand formats the command for display in the user interface.
func formatCommand(command []string) string {
	return strings.Join(command, " ")
}

type execResult struct {
	ExitCode int
	Stdout   bytes.Buffer
	Stderr   bytes.Buffer
}

func execCommand(ctx context.Context, command ...string) (*execResult, error) {
	log := klog.FromContext(ctx)

	result := &execResult{}
	stdin := ""
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout = &result.Stdout
	cmd.Stderr = &result.Stderr
	cmd.Stdin = strings.NewReader(stdin)

	log.V(2).Info("running command", "command", strings.Join(cmd.Args, " "), "stdin", stdin)

	if err := cmd.Run(); err != nil {
		log.Error(err, "running command", "stdout", result.Stdout.String(), "stderr", result.Stderr.String())
	}

	result.ExitCode = cmd.ProcessState.ExitCode()
	return result, nil
}
