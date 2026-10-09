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

package sessions

import (
	"regexp"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

var (
	invalidEnvCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
	multipleDashesRegex  = regexp.MustCompile(`-+`)
)

// SanitizeEnvironment normalizes an environment identifier.
func SanitizeEnvironment(name string) string {
	cleaned := strings.TrimSpace(name)
	cleaned = invalidEnvCharsRegex.ReplaceAllString(cleaned, "-")
	cleaned = multipleDashesRegex.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "default"
	}
	return strings.ToLower(cleaned)
}

// DetectKubeContext extracts the current-context from the kubeconfig file.
func DetectKubeContext(kubeconfigPath string) string {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	config, err := loadingRules.Load()
	if err != nil || config == nil {
		return ""
	}
	return config.CurrentContext
}

// ResolveEnvironment determines the environment scope.
// An explicit value takes precedence. If empty, it inspects kubeconfig current-context.
// Defaults to "default" when detection yields no context.
func ResolveEnvironment(explicitEnv, kubeconfigPath string) string {
	if explicitEnv != "" {
		return SanitizeEnvironment(explicitEnv)
	}

	if context := DetectKubeContext(kubeconfigPath); context != "" {
		return SanitizeEnvironment(context)
	}

	return "default"
}
