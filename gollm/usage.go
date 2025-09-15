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

package gollm

import (
	azopenai "github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	openai "github.com/openai/openai-go"
	genai "google.golang.org/genai"
)

// modelContextLimits defines approximate context window sizes for known models.
// The values are best-effort estimates and may be updated as providers change
// their offerings.  Models not listed here will return an unknown limit.
var modelContextLimits = map[string]int64{
	// OpenAI and OpenAI-compatible models
	"gpt-4o":        128000,
	"gpt-4o-mini":   128000,
	"gpt-4.1":       128000,
	"gpt-4.1-mini":  128000,
	"gpt-3.5-turbo": 16384,

	// Gemini models
	"gemini-1.5-flash":                          1048576,
	"gemini-1.5-pro":                            2097152,
	"gemini-1.5-pro-latest":                     2097152,
	"gemini-2.0-flash":                          1048576,
	"gemini-2.0-flash-preview-image-generation": 32000,
	"gemini-2.5-flash":                          1048576,
	"gemini-2.5-flash-lite":                     1048576,
	"gemini-2.5-flash-preview-04-17":            1048576,
	"gemini-2.5-flash-preview-05-20":            1048576,
	"gemini-2.5-pro":                            1048576,
	"gemini-2.5-pro-exp-03-25":                  1048576,
	"gemini-2.5-pro-preview-03-25":              1048576,
	"gemini-2.5-pro-preview-05-06":              1048576,
	"gemini-2.5-pro-preview-06-05":              1048576,
	// Grok (OpenAI compatible) models
	"grok-beta": 128000,
}

// totalTokens extracts the total token count from provider specific usage
// metadata.  It returns ok=false if the usage type is unrecognized or no token
// information is available.
func TotalTokens(usage any) (tokens int64, ok bool) {
	switch u := usage.(type) {
	case openai.CompletionUsage:
		return u.TotalTokens, true
	case azopenai.CompletionsUsage:
		if u.TotalTokens != nil {
			return int64(*u.TotalTokens), true
		}
	case genai.GenerateContentResponseUsageMetadata:
		return int64(u.TotalTokenCount), true
	case *genai.GenerateContentResponseUsageMetadata:
		if u != nil {
			return int64(u.TotalTokenCount), true
		}
	}
	return 0, false
}

// ContextPercentRemaining calculates the percentage of context window
// remaining for the given model using provider specific usage metadata.  The
// second return value indicates whether the percentage could be calculated.
func ContextPercentRemaining(model string, consumed int64) (float64, bool) {
	limit, ok := modelContextLimits[model]
	if !ok || limit == 0 {
		return 0, false
	}

	if consumed >= limit {
		return 0, true
	}

	remaining := float64(limit-consumed) / float64(limit) * 100
	return remaining, true
}
