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

package ui

import "context"

type UI interface {
	RenderOutput(ctx context.Context, s string, style ...StyleOption)
	AskForConfirmation(ctx context.Context, s string) bool

	// ClearScreen clears any output rendered to the screen
	ClearScreen()
}

type style struct {
	foreground     ColorValue
	renderMarkdown bool
}

type ColorValue string

const (
	ColorGreen ColorValue = "green"
	ColorWhite            = "white"
	ColorRed              = "red"
)

type StyleOption func(s *style)

func Foreground(color ColorValue) StyleOption {
	return func(s *style) {
		s.foreground = color
	}
}

func RenderMarkdown() StyleOption {
	return func(s *style) {
		s.renderMarkdown = true
	}
}
