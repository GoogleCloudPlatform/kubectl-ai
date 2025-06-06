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
		"help": true, "options": true, "plugin": true, "proxy": true,
		"completion": true, "convert": true, "alpha": true, "events": true,
		"port-forward": true,
	}

	writeOps = map[string]bool{
		"create": true, "apply": true, "edit": true, "delete": true,
		"patch": true, "replace": true, "scale": true, "autoscale": true,
		"expose": true, "rollout": true, "run": true, "set": true,
		"label": true, "annotate": true, "taint": true, "drain": true,
		"cordon": true, "uncordon": true, "certificate": true,
		"debug": true, "attach": true, "cp": true,
	}

	specialCases = map[string]map[string]string{
		"auth": {
			"can-i": "no", "whoami": "no", "reconcile": "yes",
		},
		"certificate": {
			"approve": "yes", "deny": "yes",
		},
		"krew": {
			"install": "yes", "list": "no",
		},
	}

	knownPlugins = map[string]string{
		"view-secret": "no", "tree": "no",
		"edit-secret": "yes", "restart": "yes",
	}
)

// KubectlModifiesResource analyzes a kubectl command to determine if it modifies resources
func KubectlModifiesResource(command string) string {
	// Normalize command
	command = normalizeCommand(command)

	// Parse command
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		klog.Errorf("Failed to parse kubectl command: %v, command: %q", err, command)
		return "unknown"
	}

	var result commandResult
	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			result.merge(analyzeCall(call))
		}
		return true
	})

	klog.V(3).Infof("KubectlModifiesResource result: %v for command: %q", result, command)
	return result.finalize()
}

type commandResult struct {
	writeFound   bool
	readFound    bool
	unknownFound bool
}

func (r *commandResult) merge(other commandResult) {
	r.writeFound = r.writeFound || other.writeFound
	r.readFound = r.readFound || other.readFound
	r.unknownFound = r.unknownFound || other.unknownFound
}

func (r commandResult) finalize() string {
	if r.writeFound {
		return "yes"
	}
	if r.unknownFound && !r.writeFound {
		return "unknown"
	}
	if r.readFound {
		return "no"
	}
	return "unknown"
}

// analyzeCall now uses the package-level constants
func analyzeCall(call *syntax.CallExpr) commandResult {
	if call == nil || len(call.Args) == 0 {
		klog.Warning("analyzeCall: call is nil or has no args")
		return commandResult{unknownFound: true}
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
		return commandResult{unknownFound: true}
	}

	// Find kubectl command
	kubectlPos := -1
	for i, arg := range args {
		if arg == "kubectl" || arg == "/usr/local/bin/kubectl" {
			kubectlPos = i
			break
		}
		if strings.Contains(arg, "kubectl") {
			klog.V(2).Infof("analyzeCall: ambiguous kubectl arg: %q", arg)
			return commandResult{unknownFound: true}
		}
	}

	if kubectlPos == -1 || kubectlPos >= len(args)-1 {
		klog.Warningf("analyzeCall: kubectl not found or no verb in args: %v", args)
		return commandResult{unknownFound: true}
	}

	// Get the verb (first non-flag argument after kubectl)
	verbPos := kubectlPos + 1
	for verbPos < len(args) && strings.HasPrefix(args[verbPos], "-") {
		verbPos++
	}

	if verbPos >= len(args) {
		klog.Warningf("analyzeCall: no verb found after kubectl in args: %v", args)
		return commandResult{unknownFound: true}
	}

	verb := args[verbPos]
	hasDryRun := hasDryRunFlag(strings.Join(args, " "))

	// Check special cases first
	if subcmds, ok := specialCases[verb]; ok && len(args) > verbPos+1 {
		if result, ok := subcmds[args[verbPos+1]]; ok {
			if result == "yes" && !hasDryRun {
				klog.V(2).Infof("analyzeCall: special case write for verb=%q subcmd=%q", verb, args[verbPos+1])
				return commandResult{writeFound: true}
			}
			klog.V(2).Infof("analyzeCall: special case read for verb=%q subcmd=%q", verb, args[verbPos+1])
			return commandResult{readFound: true}
		}
	}

	// Check plugins
	if result, ok := knownPlugins[verb]; ok {
		if result == "yes" && !hasDryRun {
			klog.V(2).Infof("analyzeCall: known plugin write for verb=%q", verb)
			return commandResult{writeFound: true}
		}
		klog.V(2).Infof("analyzeCall: known plugin read for verb=%q", verb)
		return commandResult{readFound: true}
	}

	// Check standard operations
	if writeOps[verb] && !hasDryRun {
		klog.V(2).Infof("analyzeCall: write op for verb=%q", verb)
		return commandResult{writeFound: true}
	}
	if readOnlyOps[verb] || (writeOps[verb] && hasDryRun) {
		klog.V(2).Infof("analyzeCall: read op for verb=%q (dry-run=%v)", verb, hasDryRun)
		return commandResult{readFound: true}
	}

	klog.V(1).Infof("analyzeCall: unknown op for verb=%q", verb)
	return commandResult{unknownFound: true}
}

func normalizeCommand(command string) string {
	command = strings.ReplaceAll(command, "\\n", " ")
	command = strings.ReplaceAll(command, "\n", " ")
	return strings.Join(strings.Fields(command), " ")
}

func hasDryRunFlag(command string) bool {
	tokens := strings.Fields(command)
	for i, token := range tokens {
		if token == "--dry-run" || strings.HasPrefix(token, "--dry-run=") {
			return true
		}
		if token == "--dry-run" && i < len(tokens)-1 {
			if tokens[i+1] == "client" || tokens[i+1] == "server" {
				return true
			}
		}
	}
	return false
}

// containsAny checks if the command contains any of the given strings
// For flag detection, it properly checks word boundaries to avoid false positives
func containsAny(command string, substrings []string) bool {
	// Split the command into space-separated tokens to properly detect flags
	tokens := strings.Fields(command)

	for _, substr := range substrings {
		// Simple substring check for non-flag strings
		if !strings.HasPrefix(substr, "--") {
			if strings.Contains(command, substr) {
				return true
			}
			continue
		}

		// For flags, we need to ensure proper flag matching
		for _, token := range tokens {
			if token == substr || strings.HasPrefix(token, substr+"=") {
				return true
			}
		}
	}
	return false
}
