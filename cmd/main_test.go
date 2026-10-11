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

package main

import (
	"testing"
)

func TestOptions_MemoryDefaults(t *testing.T) {
	var opt Options
	opt.InitDefaults()

	if opt.EnableMemory {
		t.Errorf("expected EnableMemory to be false by default, got true")
	}
	if opt.MemoryEnvironment != "" {
		t.Errorf("expected MemoryEnvironment to be empty by default, got %q", opt.MemoryEnvironment)
	}
	if opt.MemoryDir != "" {
		t.Errorf("expected MemoryDir to be empty by default, got %q", opt.MemoryDir)
	}
}

func TestOptions_MemoryFlagParsing(t *testing.T) {
	var opt Options
	opt.InitDefaults()

	cmd, err := BuildRootCommand(&opt)
	if err != nil {
		t.Fatalf("failed to build root command: %v", err)
	}

	err = cmd.ParseFlags([]string{
		"--enable-memory",
		"--memory-env", "staging-cluster",
		"--memory-dir", "/tmp/memory-test",
	})
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if !opt.EnableMemory {
		t.Errorf("expected EnableMemory to be true after parsing flag")
	}
	if opt.MemoryEnvironment != "staging-cluster" {
		t.Errorf("expected MemoryEnvironment to be 'staging-cluster', got %q", opt.MemoryEnvironment)
	}
	if opt.MemoryDir != "/tmp/memory-test" {
		t.Errorf("expected MemoryDir to be '/tmp/memory-test', got %q", opt.MemoryDir)
	}
}

func TestOptions_MemoryConfigLoading(t *testing.T) {
	var opt Options
	opt.InitDefaults()

	configYAML := []byte(`
enableMemory: true
memoryEnvironment: "prod-cluster"
memoryDir: "/var/data/kubectl-ai-memory"
`)

	if err := opt.LoadConfiguration(configYAML); err != nil {
		t.Fatalf("failed to load configuration: %v", err)
	}

	if !opt.EnableMemory {
		t.Errorf("expected EnableMemory to be true after loading config")
	}
	if opt.MemoryEnvironment != "prod-cluster" {
		t.Errorf("expected MemoryEnvironment to be 'prod-cluster', got %q", opt.MemoryEnvironment)
	}
	if opt.MemoryDir != "/var/data/kubectl-ai-memory" {
		t.Errorf("expected MemoryDir to be '/var/data/kubectl-ai-memory', got %q", opt.MemoryDir)
	}
}
