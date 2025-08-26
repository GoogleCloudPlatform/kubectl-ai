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
		"expose": true, "run": true, "exec": true, "set": true,
		"label": true, "annotate": true, "taint": true, "drain": true,
		"cordon": true, "uncordon": true, "debug": true, "attach": true,
		"cp": true, "reconcile": true, "approve": true, "deny": true,
		"certificate": true,
	}

	readOnlySubOps = map[string]map[string]bool{
		"rollout": {
			"history": true,
			"status":  true,
		},
	}

	writeSubOps = map[string]map[string]bool{
		"rollout": {
			"pause":   true,
			"restart": true,
			"resume":  true,
			"undo":    true,
		},
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
	numCmds := 0

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
			numCmds++
			if numCmds > 1 {
				return false // Stop walking if more then one command is found
			}
		}
		return true
	})

	if numCmds > 1 {
		// if it's a composite bash command, we should err on the side of caution and return unknown
		// to prevent exfilteration attacks https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/
		klog.Infof("KubectlModifiesResource result: unknown for command: %q, multiple commands (%d) found", command, numCmds)
		return "unknown"
	}

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
			lit = strings.Trim(sb.String(), "'\"")
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

	// Check for boolean or spaced key-value flags before the verb
	for _, arg := range args[1:] {
		if !strings.HasPrefix(arg, "-") {
			break
		}
		// If flag does not contain '=', it's boolean or spaced key-value
		if !strings.Contains(arg, "=") {
			klog.Warningf("analyzeCall: boolean or spaced key-value flag before verb: %q", arg)
			return "unknown"
		}
	}

	// Parse kubectl arguments to extract verb, subverb, and flags
	verb, subVerb, hasDryRun := parseKubectlArgs(args[1:])
	if verb == "" {
		klog.Warningf("analyzeCall: no verb found after kubectl in args: %v", args)
		return "unknown"
	}

	// Check standard operations - write operations first (prioritize immediate detection)
	if (writeOps[verb] || writeSubOps[verb][subVerb]) && !hasDryRun {
		klog.V(1).Infof("analyzeCall: write op for verb=%q subVerb=%q", verb, subVerb)
		return "yes"
	}

	// Check read-only operations or dry-run write operations
	if (readOnlyOps[verb] || readOnlySubOps[verb][subVerb]) || ((writeOps[verb] || writeSubOps[verb][subVerb]) && hasDryRun) {
		klog.V(1).Infof("analyzeCall: read op for verb=%q subVerb=%q (dry-run=%v)", verb, subVerb, hasDryRun)
		return "no"
	}

	klog.V(1).Infof("analyzeCall: unknown op for verb=%q subVerb=%q", verb, subVerb)
	return "unknown"
}

// parseKubectlArgs extracts verb, subverb, and dry-run flag from kubectl arguments
func parseKubectlArgs(args []string) (verb, subVerb string, hasDryRun bool) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--dry-run") {
			hasDryRun = true
		}
		if !strings.HasPrefix(arg, "-") {
			if verb == "" {
				verb = arg
			} else if subVerb == "" {
				subVerb = arg
			}
		}
	}
	return verb, subVerb, hasDryRun
}
