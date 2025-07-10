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
	"reflect"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// BuildSchemaFor will build a schema for the given golang type.
// Because this does not have description populated, it is more useful for the response schema than tools/functions.
func BuildSchemaFor(t reflect.Type) *Schema {
	out := &Schema{}

	switch t.Kind() {
	case reflect.String:
		out.Type = TypeString
	case reflect.Bool:
		out.Type = TypeBoolean
	case reflect.Int:
		out.Type = TypeInteger
	case reflect.Struct:
		out.Type = TypeObject
		out.Properties = make(map[string]*Schema)
		numFields := t.NumField()
		required := []string{}
		for i := 0; i < numFields; i++ {
			field := t.Field(i)
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" {
				continue
			}
			if strings.HasSuffix(jsonTag, ",omitempty") {
				jsonTag = strings.TrimSuffix(jsonTag, ",omitempty")
			} else {
				required = append(required, jsonTag)
			}

			fieldType := field.Type

			fieldSchema := BuildSchemaFor(fieldType)
			out.Properties[jsonTag] = fieldSchema
		}

		if len(required) != 0 {
			out.Required = required
		}
	case reflect.Slice:
		out.Type = TypeArray
		out.Items = BuildSchemaFor(t.Elem())
	default:
		klog.Fatalf("unhandled kind %v", t.Kind())
	}

	return out
}

// ChatMessage is a message in the chat history.
type ChatMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"`
	Content   []any     `json:"content,omitempty"`
	Response  any       `json:"response,omitempty"`
}

// RecordCompletionResponse is a standard struct for persisting LLM completion responses.
type RecordCompletionResponse struct {
	Text string `json:"text"`
	Raw  any    `json:"raw"`
}

// RecordChatResponse is a standard struct for persisting LLM chat responses.
type RecordChatResponse struct {
	// TODO: Structured data?
	Raw any `json:"raw"`
}
