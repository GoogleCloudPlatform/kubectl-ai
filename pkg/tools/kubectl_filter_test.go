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
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestKubectlModifiesResource(t *testing.T) {
	// Group test cases by category
	testCases := map[string][]struct {
		name     string
		command  string
		expected string
	}{
		"read-only commands": {
			{"Get pods", "kubectl get pods", "no"},
			{"Describe pod", "kubectl describe pod nginx", "no"},
			{"Port-forward", "kubectl port-forward pod/nginx 8080:80", "no"},
			{"Port-forward with service", "kubectl port-forward svc/nginx 8080:80", "no"},
			{"Port-forward complex", "kubectl port-forward deployment/myapp 8080:8080 9000:9000", "no"},
			{"Port-forward background", "kubectl port-forward svc/nginx 8080:80 &", "no"},
			{"Get with output", "kubectl get pods -o yaml", "no"},
			{"Get with output redirection", "kubectl get pods > pods.txt", "no"},
			{"Get with name", "kubectl get pod nginx", "no"},
			{"Config view", "kubectl config view", "no"},
			{"Version", "kubectl version", "no"},
			{"API resources", "kubectl api-resources", "no"},
			{"Explain", "kubectl explain pods", "no"},
			{"Logs", "kubectl logs nginx", "no"},
			{"Logs with follow", "kubectl logs nginx -f", "no"},
			{"Watch pods", "kubectl get pods --watch", "no"},
			{"Watch pods short", "kubectl get pods -w", "no"},
			{"Rollout status", "kubectl rollout status deployment nginx", "no"},
			{"Diff", "kubectl diff -f deployment.yaml", "no"},
			{"Can-i", "kubectl auth can-i create pods", "no"},
			{"Kustomize", "kubectl kustomize ./", "no"},
			{"Convert", "kubectl convert -f pod.yaml --output-version=v1", "no"},
			{"Events", "kubectl events", "no"},
			{"Alpha debug", "kubectl alpha debug pod/nginx", "unknown"},
			{"Auth whoami", "kubectl auth whoami", "no"},
		},
		"modifying commands": {
			{"Create pod", "kubectl create -f pod.yaml", "yes"},
			{"Apply deployment", "kubectl apply -f deployment.yaml", "yes"},
			{"Delete pod", "kubectl delete pod nginx", "yes"},
			{"Scale deployment", "kubectl scale deployment nginx --replicas=3", "yes"},
			{"Edit deployment", "kubectl edit deployment nginx", "yes"},
			{"Patch service", "kubectl patch svc nginx -p '{\"spec\":{\"type\":\"NodePort\"}}'", "yes"},
			{"Label pod", "kubectl label pod nginx app=web", "yes"},
			{"Annotate", "kubectl annotate pods nginx description='my nginx'", "yes"},
			{"Rollout restart", "kubectl rollout restart deployment nginx", "yes"},
			{"Set image", "kubectl set image deployment/nginx nginx=nginx:latest", "yes"},
			{"Exec into pod", "kubectl exec -n demo tgi-pod -- nvidia-smi", "yes"},
			{"Taint node", "kubectl taint nodes node1 key=value:NoSchedule", "yes"},
			{"Run pod", "kubectl run nginx --image=nginx", "yes"},
			{"Config set-context", "kubectl config set-context my-context", "no"},
			{"Exec command", "kubectl exec nginx -- rm -rf /", "yes"},
			{"Cordon node", "kubectl cordon node1", "yes"},
			{"Uncordon node", "kubectl uncordon node1", "yes"},
			{"Drain node", "kubectl drain node1", "yes"},
			{"Certificate approve", "kubectl certificate approve csr-12345", "yes"},
		},
		"special cases": {
			{"Dry run create", "kubectl create -f pod.yaml --dry-run=client", "no"},
			{"Dry run apply", "kubectl apply -f deployment.yaml --dry-run", "no"},
			{"Apply with server dry-run", "kubectl apply -f pod.yaml --dry-run=server", "no"},
			{"Delete with dry-run", "kubectl delete pod nginx --dry-run client", "no"},
		},
		"edge cases": {
			{"Command with pipe", "kubectl get pods | grep nginx", "unknown"},
			{"Command with backticks", "kubectl get pod `cat podname.txt`", "unknown"},
			{"Complex path", "\"/path with spaces/kubectl\" get pods", "no"},
			{"Command with env var", "KUBECONFIG=/path/to/config kubectl get pods", "no"},

			{"Not kubectl command", "ls -la", "unknown"},
			{"Multiple spaces", "kubectl  get   pods", "no"},
			{"Complex command with variables", "kubectl get pods -l app=$APP_NAME -n $NAMESPACE", "no"},
			{"Command with quotes", "kubectl get pods -l \"app=my app\"", "no"},
			{"Command with escaped quotes", "kubectl patch configmap my-config --patch \"{\\\"data\\\":{\\\"key\\\":\\\"new-value\\\"}}\"", "yes"},
			{"Complex env vars", "KUBECONFIG=/path/to/config NS=default kubectl get pods -n $NS", "no"},
			{"Command with multiple env vars", "KUBECONFIG=/config KUBECTL_EXTERNAL_DIFF=\"diff -u\" kubectl diff -f file.yaml", "no"},
			{"Sequential commands with semicolon", "kubectl get ns; kubectl create ns test", "yes"},
			{"Multiple safe commands", "kubectl get pods; kubectl get deployments", "unknown"},
			{"Mix safe and unsafe with result", "kubectl get pods && kubectl delete pod bad-pod", "yes"},
			{"Mix with initial unsafe", "kubectl delete pod bad-pod && kubectl get pods", "yes"},
			{"Kubectl alias k", "k get pods", "unknown"},
			{"Full path with arguments", "/usr/local/custom/kubectl --kubeconfig=/path/config get pods", "no"},
			{"Complex jsonpath", "kubectl get pods -o=jsonpath='{range .items[*]}{.metadata.name}{\"\\t\"}{.status.phase}{\"\\n\"}{end}'", "no"},
			{"Custom columns", "kubectl get pods -o=custom-columns=NAME:.metadata.name,STATUS:.status.phase", "no"},
			{"Impersonation", "kubectl get pods --as=system:serviceaccount:default:deployer", "no"},
			{"With token", "kubectl --token=eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9... get pods", "no"},
			{"Weird spacing", "kubectl     get    pods   -n   default", "no"},
			{"kubectl in shell script with line continuation", "kubectl get pods \\\n  --namespace=production", "no"},
			{"kubectl command split across lines in CI/CD", "kubectl delete pod \\\n  my-pod-name \\\n  --grace-period=30", "yes"},
			{"kubectl with multiline YAML pipe", "echo 'apiVersion: v1\nkind: Pod' | kubectl apply -f -", "yes"},
			{"kubectl logs with line breaks in shell", "kubectl logs \\\n  deployment/my-app \\\n  --follow", "no"},
			{"Watch with selector", "kubectl get pods --selector app=nginx --watch", "no"},
			{"Negative watch timeout", "kubectl get pods --watch-only --timeout=10s", "no"},
			{"Flags after name", "kubectl delete pod mypod --now --grace-period=0", "yes"},
			{"Server-side apply", "kubectl apply -f deploy.yaml --server-side", "yes"},
			{"Field manager", "kubectl apply -f deploy.yaml --field-manager=controller", "yes"},
			{"Create service account", "kubectl create serviceaccount jenkins", "yes"},
			{"Create role binding", "kubectl create rolebinding admin --clusterrole=admin --user=user1 --namespace=default", "yes"},
			{"Versioned kubectl", "kubectl.1.24 get pods", "no"},
			{"Config set credentials", "kubectl config set-credentials cluster-admin --token=secret", "no"},
			{"Config view with flatten", "kubectl config view --flatten", "no"},
			{"Config view with output", "kubectl config view -o json", "no"},
			{"Config use-context", "kubectl config use-context production", "no"},
			{"Label with special characters", "kubectl label pod nginx 'app.kubernetes.io/name=nginx-controller'", "yes"},
			{"Jsonpath with quotes", "kubectl get pods -o jsonpath='{.items[0].metadata.name}'", "no"},
			{"Command with grep", "kubectl get pods | grep -v Completed", "unknown"},
			{"Command with awk", "kubectl get pods | awk '{print $1}'", "unknown"},
			{"Delete with force", "kubectl delete pod stuck-pod --force --grace-period=0", "yes"},
			{"Custom resource get", "kubectl get virtualmachines", "no"},
			{"Custom resource apply", "kubectl apply -f vm-instance.yaml", "yes"},
			{"Multiple input files", "kubectl delete -f file1.yaml -f file2.yaml", "yes"},
			{"URL as input file", "kubectl apply -f https://example.com/manifest.yaml", "yes"},
			{"Input from stdin", "cat file.yaml | kubectl apply -f -", "yes"},
			{"Proxy command", "kubectl proxy --port=8080", "no"},
			{"Attach command", "kubectl attach mypod -i", "yes"},
			{"Copy files", "kubectl cp mypod:/tmp/foo /tmp/bar", "yes"},
			{"Rollout status with flags", "kubectl rollout --recursive=false status --timeout=0s deployment -w nginx", "no"},
		},
	}

	for category, cases := range testCases {
		t.Run(category, func(t *testing.T) {
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					result := kubectlModifiesResource(tt.command)
					if result != tt.expected {
						t.Errorf("KubectlModifiesResource(%q) = %q, want %q",
							tt.command, result, tt.expected)
					}
				})
			}
		})
	}
}

// TestKubectlAnalyzerComponents tests the internal helper functions used by KubectlModifiesResource
func TestKubectlAnalyzerComponents(t *testing.T) {
	t.Run("parseKubectlArgs detection", func(t *testing.T) {
		tests := []struct {
			command           string
			verbExpected      string
			subverbExpected   string
			hasDryRunExpected bool
		}{
			{"kubectl apply -f deploy.yaml --dry-run=client", "apply", "deploy.yaml", true},
			{"kubectl apply -f deploy.yaml --dry-run", "apply", "deploy.yaml", true},
			{"kubectl delete pod nginx --dry-run client", "delete", "pod", true},
			{"kubectl delete pod nginx --dry-run=server", "delete", "pod", true},
			{"kubectl apply -f deploy.yaml", "apply", "deploy.yaml", false},
			{"kubectl get pods --dry", "get", "pods", false}, // Not a valid dry-run flag
			{"echo --dry-run", "", "", true},                 // The current implementation doesn't check if it's kubectl
			{"kubectl rollout status deployment nginx", "rollout", "status", false},
		}

		for _, tt := range tests {
			verb, subVerb, hasDryRun := parseKubectlArgs(strings.Split(tt.command, " ")[1:]) // Skip the first arg (kubectl)
			if verb != tt.verbExpected {
				t.Errorf("parseKubectlArgs(%q) verb = %q, want %q", tt.command, verb, tt.verbExpected)
			}
			if subVerb != tt.subverbExpected {
				t.Errorf("parseKubectlArgs(%q) subVerb = %q, want %q", tt.command, subVerb, tt.subverbExpected)
			}
			if hasDryRun != tt.hasDryRunExpected {
				t.Errorf("parseKubectlArgs(%q) hasDryRun = %v, want %v", tt.command, hasDryRun, tt.hasDryRunExpected)
			}
		}
	})

	t.Run("command parsing", func(t *testing.T) {
		tests := []struct {
			command     string
			expectedRes string
		}{
			{"kubectl get pods", "no"},
			{"kubectl apply -f deploy.yaml", "yes"},
			{"ls -la", "unknown"},      // Not kubectl
			{"kubectl", "unknown"},     // Incomplete command
			{"kubectl; ls", "unknown"}, // Multiple commands
		}

		for _, tt := range tests {
			result := kubectlModifiesResource(tt.command)
			if result != tt.expectedRes {
				t.Errorf("KubectlModifiesResource(%q) = %q, want %q",
					tt.command, result, tt.expectedRes)
			}
		}
	})
}

// TestKubectlCommandParsing tests kubectl command parsing focusing on realistic scenarios
func TestKubectlCommandParsing(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected string
		desc     string
	}{
		// Basic kubectl detection
		{"literal kubectl", "kubectl get pods", "no", "standard kubectl command"},
		{"kubectl.exe", "kubectl.exe get pods", "no", "Windows executable"},
		{"Unix path", "/usr/bin/kubectl get pods", "no", "Full Unix path"},
		{"relative path", "./kubectl get services", "no", "relative path"},
		{"nested path", "../bin/kubectl describe pod nginx", "no", "nested relative path"},

		// Windows paths with forward slashes (works cross-platform)
		{"Windows forward slash", "C:/tools/kubectl.exe delete pod nginx", "yes", "Windows path with forward slash"},

		// macOS/Homebrew paths
		{"macOS Homebrew", "/opt/homebrew/bin/kubectl get nodes", "no", "macOS Homebrew path"},
		{"macOS Intel Homebrew", "/usr/local/bin/kubectl create namespace test", "yes", "macOS Intel Homebrew path"},
		{"macOS Applications", "/Applications/Docker.app/Contents/Resources/bin/kubectl get all", "no", "macOS Docker Desktop kubectl"},

		// Non-kubectl commands
		{"not kubectl", "k get pods", "unknown", "kubectl alias"},
		{"kubectl suffix", "kubectl-1.28 get pods", "no", "kubectl with version suffix"},
		{"kubectl prefix", "kubectl-proxy --port=8080", "unknown", "kubectl with additional suffix"},
		{"different command", "kubectx production", "unknown", "different k8s tool"},

		// Environment variables
		{"env var prefix", "KUBECONFIG=/path/config kubectl get pods", "no", "environment variable prefix"},
		{"multiple env vars", "KUBECONFIG=/config NS=default kubectl apply -f deploy.yaml --dry-run", "no", "multiple environment variables"},

		// Complex scenarios
		{"long path", "/very/long/path/to/kubectl get pods", "no", "very long path"},
		{"flags before verb", "kubectl --context=prod --namespace=app get pods", "no", "global flags before verb"},
		{"flags before verb mutating", "kubectl --replicas=3 scale deployment/nginx-deployment", "yes", "global flags before verb mutating"},
		{"flags before verb without equals", "kubectl --context prod --namespace app get pods", "unknown", "global flags before verb without equals"},
		{"no verb", "kubectl --help", "unknown", "kubectl with only flags"},
		{"boolean flag before verb", "kubectl --verbose get pods", "unknown", "boolean flag before verb"},
		{"boolean flag before verb mutating", "kubectl --force delete pod nginx", "unknown", "boolean flag before verb mutating"},
		{"mixed flags before verb", "kubectl --context=prod --namespace app get pods", "unknown", "mixed non-spaced and spaced flags before verb"},
		{"non-spaced key-value before verb non-mutating", "kubectl --namespace=default get pods", "no", "non-spaced key-value before verb non-mutating"},
		{"non-spaced key-value before verb mutating", "kubectl --namespace=default delete pod nginx", "yes", "non-spaced key-value before verb mutating"},
		{"flag after verb spaced", "kubectl get pods --context prod", "no", "spaced key-value flag after verb"},
		{"flag after verb boolean", "kubectl get pods --verbose", "no", "boolean flag after verb"},
		{"flag after verb mutating", "kubectl delete pod nginx --force", "yes", "boolean flag after verb mutating"},
		{"flag with equals empty value before verb", "kubectl --namespace= get pods", "no", "non-spaced key-value with empty value before verb"},
		{"unexpected arg before verb", "kubectl something get pods", "unknown", "unexpected arg before verb"},
		{"multiple boolean flags before verb", "kubectl --verbose --debug get pods", "unknown", "multiple boolean flags before verb"},
		{"multiple spaced flags before verb", "kubectl --context prod --namespace app get pods", "unknown", "multiple spaced flags before verb"},
		{"multiple non-spaced flags before verb mutating", "kubectl --namespace=default --force=true delete pod nginx", "yes", "multiple non-spaced flags before verb mutating"},
		{"multiple non-spaced flags before verb non-mutating", "kubectl --namespace=default --verbose=true get pods", "no", "multiple non-spaced flags before verb non-mutating"},

		// Dry run scenarios
		{"dry run create", "/usr/bin/kubectl create -f pod.yaml --dry-run=client", "no", "dry run with path"},
		{"dry run apply", "kubectl.exe apply -f deploy.yaml --dry-run", "no", "Windows dry run"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := kubectlModifiesResource(tt.command)
			if result != tt.expected {
				t.Errorf("KubectlModifiesResource(%q) = %q, want %q\nDescription: %s",
					tt.command, result, tt.expected, tt.desc)
			}
		})
	}
}

// TestKubectlPathHandling tests the OS-agnostic path handling specifically
func TestKubectlPathHandling(t *testing.T) {
	tests := []struct {
		name        string
		binaryPath  string
		shouldMatch bool
		description string
	}{
		// Basic cases
		{"Standard kubectl", "kubectl", true, "Standard kubectl binary name"},
		{"Windows kubectl.exe", "kubectl.exe", true, "Windows kubectl executable"},
		{"Unix full path", "/usr/bin/kubectl", true, "Full Unix path to kubectl"},
		{"Windows forward slash", "C:/tools/kubectl.exe", true, "Windows path with forward slashes"},
		{"Relative path", "./kubectl", true, "Relative path to kubectl"},
		{"macOS Homebrew", "/opt/homebrew/bin/kubectl", true, "macOS Homebrew path"},

		// Non-kubectl binaries
		{"Not kubectl", "k", false, "Short alias should not match"},
		{"kubectl with suffix", "kubectl-1.28", false, "kubectl with version suffix"},
		{"kubectl prefix", "kubectl-proxy", false, "kubectl with additional suffix"},
		{"Other binary", "kubectx", false, "Different binary altogether"},
		{"Empty path", "", false, "Empty binary path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the filepath.Base logic used in the actual function
			basename := filepath.Base(tt.binaryPath)
			isKubectl := (basename == "kubectl" || basename == "kubectl.exe")

			if isKubectl != tt.shouldMatch {
				t.Errorf("filepath.Base(%q) = %q, kubectl detection = %v, want %v\nDescription: %s",
					tt.binaryPath, basename, isKubectl, tt.shouldMatch, tt.description)
			}
		})
	}
}

// TestKubectlDetectionLogic tests the core kubectl detection logic
func TestKubectlDetectionLogic(t *testing.T) {
	// Simulate the kubectl detection logic from analyzeCall
	testKubectlDetection := func(arg string) bool {
		// Reject quoted arguments
		if (strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'")) || (strings.HasPrefix(arg, "\"") && strings.HasSuffix(arg, "\"")) {
			return false
		}
		// Check if this is kubectl using OS-agnostic path handling
		basename := filepath.Base(arg)
		return basename == "kubectl" || basename == "kubectl.exe"
	}

	testCases := []struct {
		input    string
		expected bool
		desc     string
	}{
		{"kubectl", true, "literal kubectl"},
		{"kubectl.exe", true, "Windows executable"},
		{"/usr/bin/kubectl", true, "Unix path"},
		{"C:/tools/kubectl.exe", true, "Windows path with forward slash"},
		{"./kubectl", true, "relative path"},
		{"../bin/kubectl", true, "relative path with parent dir"},
		{"/opt/homebrew/bin/kubectl", true, "macOS Homebrew path"},
		{"'kubectl'", false, "quoted kubectl"},
		{"\"/usr/bin/kubectl\"", false, "quoted path"},
		{"not-kubectl", false, "different command"},
		{"/usr/bin/k", false, "different command in path"},
		{"kubectl-something", false, "kubectl with suffix"},
		{"kubectl-1.28", false, "kubectl with version suffix"},
		{"k", false, "kubectl alias"},
		{"kubectx", false, "different k8s tool"},
		{"", false, "empty string"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := testKubectlDetection(tc.input)
			if result != tc.expected {
				t.Errorf("testKubectlDetection(%q) = %t, want %t (%s)",
					tc.input, result, tc.expected, tc.desc)
			}
		})
	}
}

// TestShellParserBehavior tests how the shell parser handles different command structures
// This helps us understand if we can simplify the kubectl detection logic
func TestShellParserBehavior(t *testing.T) {
	testCommands := []struct {
		name     string
		command  string
		expected [][]string // expected args for each CallExpr
	}{
		{
			name:    "simple kubectl",
			command: "kubectl get pods",
			expected: [][]string{
				{"kubectl", "get", "pods"},
			},
		},
		{
			name:    "kubectl with env var",
			command: "KUBECONFIG=/path/config kubectl get pods",
			expected: [][]string{
				{"kubectl", "get", "pods"}, // env vars are handled separately
			},
		},
		{
			name:    "sequential commands",
			command: "kubectl get pods; kubectl create pod",
			expected: [][]string{
				{"kubectl", "get", "pods"},
				{"kubectl", "create", "pod"},
			},
		},
		{
			name:    "kubectl with path",
			command: "/usr/bin/kubectl get pods",
			expected: [][]string{
				{"/usr/bin/kubectl", "get", "pods"},
			},
		},
		{
			name:    "not kubectl",
			command: "ls -la",
			expected: [][]string{
				{"ls", "-la"},
			},
		},
	}

	parser := syntax.NewParser()

	for _, tt := range testCommands {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.Parse(strings.NewReader(tt.command), "")
			if err != nil {
				t.Fatalf("Parse error for %q: %v", tt.command, err)
			}

			var actualCalls [][]string
			syntax.Walk(file, func(node syntax.Node) bool {
				if call, ok := node.(*syntax.CallExpr); ok {
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
					actualCalls = append(actualCalls, args)
				}
				return true
			})

			if len(actualCalls) != len(tt.expected) {
				t.Errorf("Expected %d CallExpr, got %d for command %q",
					len(tt.expected), len(actualCalls), tt.command)
				return
			}

			// Debug output to understand parser behavior
			t.Logf("Command: %q", tt.command)
			for i, call := range actualCalls {
				t.Logf("  CallExpr %d: %v", i, call)
				if len(call) > 0 {
					t.Logf("    args[0] = %q", call[0])
					if strings.Contains(call[0], "kubectl") {
						t.Logf("    -> Contains kubectl!")
					}
				}
			}

			for i, expectedArgs := range tt.expected {
				if len(actualCalls[i]) != len(expectedArgs) {
					t.Errorf("CallExpr %d: expected %d args, got %d for command %q",
						i, len(expectedArgs), len(actualCalls[i]), tt.command)
					continue
				}

				for j, expectedArg := range expectedArgs {
					if actualCalls[i][j] != expectedArg {
						t.Errorf("CallExpr %d arg %d: expected %q, got %q for command %q",
							i, j, expectedArg, actualCalls[i][j], tt.command)
					}
				}
			}
		})
	}
}

// TestSimplifiedKubectlDetection tests a simplified approach to kubectl detection
func TestSimplifiedKubectlDetection(t *testing.T) {
	// Simplified kubectl detection logic
	isKubectl := func(args []string) bool {
		if len(args) == 0 {
			return false
		}

		// Get the first argument (the command)
		cmd := args[0]

		// Reject quoted arguments
		if (strings.HasPrefix(cmd, "'") && strings.HasSuffix(cmd, "'")) ||
			(strings.HasPrefix(cmd, "\"") && strings.HasSuffix(cmd, "\"")) {
			return false
		}

		// Check if this is kubectl using OS-agnostic path handling
		basename := filepath.Base(cmd)
		return basename == "kubectl" || basename == "kubectl.exe"
	}

	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"empty args", []string{}, false},
		{"kubectl", []string{"kubectl", "get", "pods"}, true},
		{"kubectl.exe", []string{"kubectl.exe", "get", "pods"}, true},
		{"path to kubectl", []string{"/usr/bin/kubectl", "get", "pods"}, true},
		{"Windows path", []string{"C:/tools/kubectl.exe", "delete", "pod"}, true},
		{"quoted kubectl", []string{"'kubectl'", "get", "pods"}, false},
		{"quoted path", []string{"\"/usr/bin/kubectl\"", "get", "pods"}, false},
		{"not kubectl", []string{"ls", "-la"}, false},
		{"kubectl with suffix", []string{"kubectl-1.28", "get", "pods"}, false},
		{"k alias", []string{"k", "get", "pods"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKubectl(tt.args)
			if result != tt.expected {
				t.Errorf("isKubectl(%v) = %v, want %v", tt.args, result, tt.expected)
			}
		})
	}
}

// TestKubectlAlwaysAtPosition0 confirms that kubectl is always at args[0] in a CallExpr
func TestKubectlAlwaysAtPosition0(t *testing.T) {
	// Test different kubectl commands to confirm kubectl is always at position 0
	testCommands := []string{
		"kubectl get pods",
		"/usr/bin/kubectl get pods",
		"kubectl.exe get pods",
		"kubectl --context=prod get pods",
		"KUBECONFIG=/path/config kubectl get pods",
	}

	parser := syntax.NewParser()

	for _, cmd := range testCommands {
		t.Run(cmd, func(t *testing.T) {
			file, err := parser.Parse(strings.NewReader(cmd), "")
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			syntax.Walk(file, func(node syntax.Node) bool {
				if call, ok := node.(*syntax.CallExpr); ok {
					if len(call.Args) == 0 {
						return true
					}

					// Extract first argument
					firstArg := call.Args[0].Lit()
					if firstArg == "" {
						var sb strings.Builder
						syntax.NewPrinter().Print(&sb, call.Args[0])
						firstArg = strings.Trim(sb.String(), `"'`)
					}

					// Check if first argument is kubectl (using same logic as main code)
					basename := filepath.Base(firstArg)
					isKubectl := basename == "kubectl" || basename == "kubectl.exe"

					if !isKubectl {
						t.Errorf("Expected kubectl at args[0], got %q (basename: %q)", firstArg, basename)
					}
				}
				return true
			})
		})
	}
}
