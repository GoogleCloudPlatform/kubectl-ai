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
	"strings"

	"k8s.io/klog/v2"
	"mvdan.cc/sh/v3/syntax"
)

// Package-level constants for kubectl operations
var (
	readOnlyOps = map[string]bool{
		"get": true, "describe": true, "explain": true, "top": true,
		"logs": true, "api-resources": true, "api-versions": true,
		"version": true, "config": true, "cluster-info": true,
		"wait": true, "auth": true, "diff": true, "kustomize": true,
		"help": true, "options": true, "proxy": true,
		"completion": true, "convert": true, "events": true,
		"port-forward": true, "can-i": true, "whoami": true,
	}

	writeOps = map[string]bool{
		"create": true, "apply": true, "edit": true, "delete": true,
		"patch": true, "replace": true, "scale": true, "autoscale": true,
		"expose": true, "rollout": true, "run": true, "set": true,
		"label": true, "annotate": true, "taint": true, "drain": true,
		"cordon": true, "uncordon": true, "debug": true, "attach": true,
		"cp": true, "reconcile": true, "approve": true, "deny": true,
		"certificate": true,
	}
)

// KubectlModifiesResource analyzes a kubectl command to determine if it modifies resources
func kubectlModifiesResource(command string) string {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		klog.Errorf("Failed to parse kubectl command: %v, command: %q", err, command)
		return "unknown"
	}

	hasReadCommand := false
	foundWrite := false

	// Single pass through all command calls
	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			result := analyzeCall(call)

			// If we find any write operation, mark it and stop
			if result == "yes" {
				foundWrite = true
				return false // Stop walking
			}

			// Track if we found any read operations
			if result == "no" {
				hasReadCommand = true
			}
		}
		return true
	})

	// Return results based on what we found
	if foundWrite {
		klog.Infof("KubectlModifiesResource result: yes (write operation found) for command: %q", command)
		return "yes"
	}

	if hasReadCommand {
		klog.Infof("KubectlModifiesResource result: no (read-only) for command: %q", command)
		return "no"
	}

	// Default to unknown if no recognized kubectl commands found
	klog.Infof("KubectlModifiesResource result: unknown for command: %q", command)
	return "unknown"
}

func analyzeCall(call *syntax.CallExpr) string {
	if call == nil || len(call.Args) == 0 {
		klog.Warning("analyzeCall: call is nil or has no args")
		return "unknown"
	}

	// Extract command and arguments
	var args []string
	for _, arg := range call.Args {
		lit := arg.Lit()
		if lit == "" {
			var sb strings.Builder
			syntax.NewPrinter().Print(&sb, arg)
			lit = strings.Trim(sb.String(), `"'`)
		}
		if lit != "" {
			args = append(args, lit)
		}
	}

	if len(args) == 0 {
		klog.Warning("analyzeCall: no arguments extracted from call")
		return "unknown"
	}

	// Check if first argument is kubectl
	firstArg := args[0]

	// Reject quoted arguments (e.g., '"/path/kubectl"')
	if (strings.HasPrefix(firstArg, "'") && strings.HasSuffix(firstArg, "'")) || (strings.HasPrefix(firstArg, "\"") && strings.HasSuffix(firstArg, "\"")) {
		klog.V(2).Infof("analyzeCall: first arg is quoted: %q", firstArg)
		return "unknown"
	}

	// Check if this is kubectl
	if !strings.Contains(firstArg, "kubectl") {
		klog.V(2).Infof("analyzeCall: first arg does not contain kubectl: %q", firstArg)
		return "unknown"
	}

	klog.V(2).Infof("analyzeCall: found kubectl: %q", firstArg)

	// Get the verb (first non-flag argument after kubectl)
	verbPos := 1 // Start after kubectl at position 0
	for verbPos < len(args) && strings.HasPrefix(args[verbPos], "-") {
		verbPos++
	}

	if verbPos >= len(args) {
		klog.Warningf("analyzeCall: no verb found after kubectl in args: %v", args)
		return "unknown"
	}

	verb := args[verbPos]
	hasDryRun := hasDryRunFlag(strings.Join(args, " "))

	// Check standard operations - write operations first (prioritize immediate detection)
	if writeOps[verb] && !hasDryRun {
		klog.V(1).Infof("analyzeCall: write op for verb=%q", verb)
		return "yes"
	}

	// Check read-only operations or dry-run write operations
	if readOnlyOps[verb] || (writeOps[verb] && hasDryRun) {
		klog.V(1).Infof("analyzeCall: read op for verb=%q (dry-run=%v)", verb, hasDryRun)
		return "no"
	}

	klog.V(1).Infof("analyzeCall: unknown op for verb=%q", verb)
	return "unknown"
}

func hasDryRunFlag(command string) bool {
	tokens := strings.Fields(command)
	for _, token := range tokens {
		if strings.HasPrefix(token, "--dry-run") {
			return true
		}
	}
	return false
}
