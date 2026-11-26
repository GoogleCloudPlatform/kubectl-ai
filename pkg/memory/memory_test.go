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

package memory

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMemoryRetentionWhenEnabled(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "memory")

	cfg := Config{
		Enabled:     true,
		Environment: "staging-cluster",
		StorageDir:  storageDir,
	}

	mgr1, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if !mgr1.IsEnabled() {
		t.Fatalf("expected manager to be enabled")
	}

	err = mgr1.Set("storage-class", "fast-nvme", "preference")
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	err = mgr1.Set("default-namespace", "backend", "guideline")
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	promptContext1 := mgr1.BuildPromptContext()
	if !strings.Contains(promptContext1, "fast-nvme") || !strings.Contains(promptContext1, "backend") {
		t.Fatalf("unexpected prompt context: %s", promptContext1)
	}

	// Create a new manager instance pointing to the same storage directory and environment
	mgr2, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create second manager: %v", err)
	}

	entry1, ok := mgr2.Get("storage-class")
	if !ok || entry1.Content != "fast-nvme" || entry1.Category != "preference" {
		t.Fatalf("expected retained storage-class entry, got: %+v (ok=%v)", entry1, ok)
	}

	entry2, ok := mgr2.Get("default-namespace")
	if !ok || entry2.Content != "backend" || entry2.Category != "guideline" {
		t.Fatalf("expected retained default-namespace entry, got: %+v (ok=%v)", entry2, ok)
	}

	promptContext2 := mgr2.BuildPromptContext()
	if promptContext2 != promptContext1 {
		t.Fatalf("prompt context mismatch: expected %q, got %q", promptContext1, promptContext2)
	}

	entries := mgr2.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestZeroPersistenceWhenDisabled(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "should-not-exist")

	cfg := Config{
		Enabled:     false,
		Environment: "production-cluster",
		StorageDir:  storageDir,
	}

	mgr1, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if mgr1.IsEnabled() {
		t.Fatalf("expected manager to be disabled")
	}

	// Operations should not fail, but must not persist to disk
	err = mgr1.Set("secret-note", "should-never-be-saved")
	if err != nil {
		t.Fatalf("set on disabled manager returned error: %v", err)
	}

	promptContext := mgr1.BuildPromptContext()
	if promptContext != "" {
		t.Fatalf("expected empty prompt context when disabled, got %q", promptContext)
	}

	// Storage directory must NOT exist on disk
	if _, err := os.Stat(storageDir); !os.IsNotExist(err) {
		t.Fatalf("storage directory should not exist when disabled, err: %v", err)
	}

	// Another manager reading the environment must see zero entries
	mgr2, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create second manager: %v", err)
	}

	if len(mgr2.List()) != 0 {
		t.Fatalf("expected zero entries on fresh load when disabled, got %d", len(mgr2.List()))
	}
}

func TestEnvironmentIsolation(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "memory")

	cfgClusterA := Config{
		Enabled:     true,
		Environment: "cluster-alpha",
		StorageDir:  storageDir,
	}

	cfgClusterB := Config{
		Enabled:     true,
		Environment: "cluster-beta",
		StorageDir:  storageDir,
	}

	mgrA, err := NewDefaultManager(cfgClusterA, "")
	if err != nil {
		t.Fatalf("failed to create cluster-alpha manager: %v", err)
	}

	mgrB, err := NewDefaultManager(cfgClusterB, "")
	if err != nil {
		t.Fatalf("failed to create cluster-beta manager: %v", err)
	}

	if err := mgrA.Set("ingress-controller", "traefik"); err != nil {
		t.Fatalf("failed to set entry in cluster-alpha: %v", err)
	}

	if err := mgrB.Set("ingress-controller", "nginx"); err != nil {
		t.Fatalf("failed to set entry in cluster-beta: %v", err)
	}

	entryA, okA := mgrA.Get("ingress-controller")
	if !okA || entryA.Content != "traefik" {
		t.Fatalf("expected traefik in cluster-alpha, got: %+v", entryA)
	}

	entryB, okB := mgrB.Get("ingress-controller")
	if !okB || entryB.Content != "nginx" {
		t.Fatalf("expected nginx in cluster-beta, got: %+v", entryB)
	}
}

func TestSanitizeEnvironmentName(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "memory")

	// Environment name containing slashes and special characters
	complexEnv := "gke_my-project_us-central1-a_prod/cluster:v1"

	cfg := Config{
		Enabled:     true,
		Environment: complexEnv,
		StorageDir:  storageDir,
	}

	mgr, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create manager with complex env name: %v", err)
	}

	if err := mgr.Set("policy", "enforce-psp"); err != nil {
		t.Fatalf("failed to save entry: %v", err)
	}

	// Verify persistence and reloading
	mgrReloaded, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to reload manager: %v", err)
	}

	entry, ok := mgrReloaded.Get("policy")
	if !ok || entry.Content != "enforce-psp" {
		t.Fatalf("expected retained policy, got: %+v", entry)
	}

	// Verify file was written inside storageDir without escaping the directory
	files, err := os.ReadDir(storageDir)
	if err != nil {
		t.Fatalf("failed to read storage dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file in storage dir, got %d", len(files))
	}
}

func TestDeleteAndClear(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "memory")

	cfg := Config{
		Enabled:     true,
		Environment: "test-cluster",
		StorageDir:  storageDir,
	}

	mgr, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	_ = mgr.Set("k1", "v1")
	_ = mgr.Set("k2", "v2")

	if len(mgr.List()) != 2 {
		t.Fatalf("expected 2 entries")
	}

	if err := mgr.Delete("k1"); err != nil {
		t.Fatalf("failed to delete k1: %v", err)
	}

	if _, ok := mgr.Get("k1"); ok {
		t.Fatalf("expected k1 to be removed")
	}

	if len(mgr.List()) != 1 {
		t.Fatalf("expected 1 entry left")
	}

	if err := mgr.Clear(); err != nil {
		t.Fatalf("failed to clear manager: %v", err)
	}

	if len(mgr.List()) != 0 {
		t.Fatalf("expected 0 entries after clear")
	}

	// After clear, reloading should also return empty
	reloaded, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to reload manager: %v", err)
	}
	if len(reloaded.List()) != 0 {
		t.Fatalf("expected 0 entries in reloaded manager")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "memory")

	cfg := Config{
		Enabled:     true,
		Environment: "concurrency-test",
		StorageDir:  storageDir,
	}

	mgr, err := NewDefaultManager(cfg, "")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "shared-key"
				_ = mgr.Set(key, "worker-value")
				_, _ = mgr.Get(key)
				_ = mgr.List()
				_ = mgr.BuildPromptContext()
			}
		}(i)
	}

	wg.Wait()

	if _, ok := mgr.Get("shared-key"); !ok {
		t.Fatalf("expected shared-key to exist after concurrent writes")
	}
}
