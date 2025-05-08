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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func init() {
	RegisterTool(&BashTool{})
}

const (
	bashBin = "/bin/bash"
)

// expandShellVar expands shell variables and syntax using bash
func expandShellVar(value string) (string, error) {
	if strings.Contains(value, "~") {
		if len(value) >= 2 && value[0] == '~' && os.IsPathSeparator(value[1]) {
			if runtime.GOOS == "windows" {
				value = filepath.Join(os.Getenv("USERPROFILE"), value[2:])
			} else {
				value = filepath.Join(os.Getenv("HOME"), value[2:])
			}
		}
	}
	return os.ExpandEnv(value), nil
}

type BashTool struct{}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Executes a bash command. Use this tool only when you need to execute a shell command."
}

func (t *BashTool) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {
					Type:        gollm.TypeString,
					Description: `The bash command to execute.`,
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

func (t *BashTool) Run(ctx context.Context, args map[string]any) (any, error) {
	kubeconfig := ctx.Value("kubeconfig").(string)
	workDir := ctx.Value("work_dir").(string)
	command := args["command"].(string)

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
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	if kubeconfig != "" {
		kubeconfig, err := expandShellVar(kubeconfig)
		if err != nil {
			return nil, err
		}
		cmd.Env = append(cmd.Env, "KUBECONFIG="+kubeconfig)
	}

	return executeCommand(cmd)
}

type ExecResult struct {
	Error    string `json:"error,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

func executeCommand(cmd *exec.Cmd) (*ExecResult, error) {
	// Detect streaming commands (e.g., kubectl logs -f, tail -f)
	isStreaming := false
	for _, arg := range cmd.Args {
		if arg == "-f" || arg == "--follow" {
			isStreaming = true
			break
		}
	}

	results := &ExecResult{}

	if isStreaming {
		// Print streaming message
		fmt.Fprintln(os.Stdout, "\nStreaming output... Press CTRL-C to stop streaming and return to the prompt.")

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			results.Error = err.Error()
			return results, err
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			results.Error = err.Error()
			return results, err
		}

		if err := cmd.Start(); err != nil {
			results.Error = err.Error()
			return results, err
		}

		// Channel to catch interrupt
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		defer signal.Stop(interrupt)

		// Read stdout and stderr concurrently
		stdoutDone := make(chan struct{})
		stderrDone := make(chan struct{})

		go func() {
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				fmt.Fprintln(os.Stdout, scanner.Text())
			}
			stdoutDone <- struct{}{}
		}()
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				fmt.Fprintln(os.Stderr, scanner.Text())
			}
			stderrDone <- struct{}{}
		}()

		// Wait for either interrupt or process exit
		select {
		case <-interrupt:
			cmd.Process.Signal(os.Interrupt)
			results.Error = "streaming interrupted by user"
			// Wait for goroutines to finish
			<-stdoutDone
			<-stderrDone
			return results, nil
		case <-stdoutDone:
			<-stderrDone
			cmd.Wait()
			return results, nil
		case <-stderrDone:
			<-stdoutDone
			cmd.Wait()
			return results, nil
		}
	}
	// Non-streaming: collect output as before
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			results.ExitCode = exitError.ExitCode()
		} else {
			return nil, err
		}
	}
	results.Stdout = stdout.String()
	results.Stderr = stderr.String()
	return results, nil
}
