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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"sigs.k8s.io/yaml"
)

func TestEnvironmentStorage_MemoryBackend(t *testing.T) {
	manager, err := NewSessionManager("memory", WithEnvironment("cluster-alpha"))
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	meta := Metadata{
		ProviderID: "gemini",
		ModelID:    "gemini-2.5-pro",
	}

	session, err := manager.NewSession(meta)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	if session.Environment != "cluster-alpha" {
		t.Errorf("expected session environment to be %q, got %q", "cluster-alpha", session.Environment)
	}

	found, err := manager.FindSessionByID(session.ID)
	if err != nil {
		t.Fatalf("FindSessionByID failed: %v", err)
	}
	if found.Environment != "cluster-alpha" {
		t.Errorf("expected found session environment to be %q, got %q", "cluster-alpha", found.Environment)
	}
}

func TestEnvironmentStorage_FilesystemBackend(t *testing.T) {
	tmpDir := t.TempDir()
	store := newFilesystemStore(tmpDir)
	manager, err := NewSessionManager("", WithStore(store), WithEnvironment("gke-prod"))
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	meta := Metadata{
		ProviderID: "gemini",
		ModelID:    "gemini-2.5-pro",
	}

	session, err := manager.NewSession(meta)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Verify metadata.yaml on disk contains environment
	metadataPath := filepath.Join(tmpDir, session.ID, "metadata.yaml")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata.yaml: %v", err)
	}

	var diskMeta Metadata
	if err := yaml.Unmarshal(data, &diskMeta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}
	if diskMeta.Environment != "gke-prod" {
		t.Errorf("expected disk metadata environment to be %q, got %q", "gke-prod", diskMeta.Environment)
	}

	// Create a new manager instance and verify it reads the stored session with environment
	newStore := newFilesystemStore(tmpDir)
	newManager, err := NewSessionManager("", WithStore(newStore), WithEnvironment("gke-prod"))
	if err != nil {
		t.Fatalf("re-creating SessionManager failed: %v", err)
	}

	loaded, err := newManager.FindSessionByID(session.ID)
	if err != nil {
		t.Fatalf("FindSessionByID on new manager failed: %v", err)
	}
	if loaded.Environment != "gke-prod" {
		t.Errorf("expected loaded session environment to be %q, got %q", "gke-prod", loaded.Environment)
	}
}

func TestEnvironmentIsolation(t *testing.T) {
	store := newMemoryStore()

	mgrProd, err := NewSessionManager("", WithStore(store), WithEnvironment("prod-cluster"))
	if err != nil {
		t.Fatalf("failed to create prod manager: %v", err)
	}
	mgrDev, err := NewSessionManager("", WithStore(store), WithEnvironment("dev-cluster"))
	if err != nil {
		t.Fatalf("failed to create dev manager: %v", err)
	}

	// Create two sessions in prod
	sProd1, err := mgrProd.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatalf("creating prod session 1: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	sProd2, err := mgrProd.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatalf("creating prod session 2: %v", err)
	}

	// Create two sessions in dev
	time.Sleep(5 * time.Millisecond)
	sDev1, err := mgrDev.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatalf("creating dev session 1: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	sDev2, err := mgrDev.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatalf("creating dev session 2: %v", err)
	}

	// Test 1: ListSessions isolation
	prodSessions, err := mgrProd.ListSessions()
	if err != nil {
		t.Fatalf("mgrProd.ListSessions failed: %v", err)
	}
	if len(prodSessions) != 2 {
		t.Fatalf("expected 2 prod sessions, got %d", len(prodSessions))
	}
	for _, s := range prodSessions {
		if s.Environment != "prod-cluster" {
			t.Errorf("leak detected: session %s in prod list has environment %q", s.ID, s.Environment)
		}
	}

	devSessions, err := mgrDev.ListSessions()
	if err != nil {
		t.Fatalf("mgrDev.ListSessions failed: %v", err)
	}
	if len(devSessions) != 2 {
		t.Fatalf("expected 2 dev sessions, got %d", len(devSessions))
	}
	for _, s := range devSessions {
		if s.Environment != "dev-cluster" {
			t.Errorf("leak detected: session %s in dev list has environment %q", s.ID, s.Environment)
		}
	}

	// Test 2: GetLatestSession isolation
	latestProd, err := mgrProd.GetLatestSession()
	if err != nil {
		t.Fatalf("mgrProd.GetLatestSession failed: %v", err)
	}
	if latestProd == nil || latestProd.ID != sProd2.ID {
		t.Errorf("expected latest prod session %s, got %v", sProd2.ID, latestProd)
	}

	latestDev, err := mgrDev.GetLatestSession()
	if err != nil {
		t.Fatalf("mgrDev.GetLatestSession failed: %v", err)
	}
	if latestDev == nil || latestDev.ID != sDev2.ID {
		t.Errorf("expected latest dev session %s, got %v", sDev2.ID, latestDev)
	}

	// Test 3: FindSessionByID cross-environment isolation enforcement
	_, err = mgrProd.FindSessionByID(sDev1.ID)
	if err == nil {
		t.Errorf("expected error when accessing dev session %s from prod manager, but got nil", sDev1.ID)
	}
	if !strings.Contains(err.Error(), "isolated from") {
		t.Errorf("expected isolation error message, got: %v", err)
	}

	// Test 4: DeleteSession cross-environment isolation enforcement
	err = mgrProd.DeleteSession(sDev1.ID)
	if err == nil {
		t.Errorf("expected error deleting dev session %s from prod manager, but got nil", sDev1.ID)
	}

	// Test 5: ListAllSessions returns all 4
	allSessions, err := mgrProd.ListAllSessions()
	if err != nil {
		t.Fatalf("ListAllSessions failed: %v", err)
	}
	if len(allSessions) != 4 {
		t.Errorf("expected 4 total sessions in ListAllSessions, got %d", len(allSessions))
	}

	_ = sProd1
}

func TestSessionEviction_MaxSessions(t *testing.T) {
	store := newMemoryStore()
	policy := RetentionPolicy{
		MaxSessions: 2,
	}

	mgr, err := NewSessionManager("", WithStore(store), WithEnvironment("cluster-test"), WithRetentionPolicy(policy))
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	// Create 4 sessions sequentially
	s1, _ := mgr.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	time.Sleep(10 * time.Millisecond)
	s2, _ := mgr.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	time.Sleep(10 * time.Millisecond)
	s3, _ := mgr.NewSession(Metadata{ProviderID: "p", ModelID: "m"})
	time.Sleep(10 * time.Millisecond)
	s4, _ := mgr.NewSession(Metadata{ProviderID: "p", ModelID: "m"})

	// Also create 1 session in a different environment
	otherMgr, _ := NewSessionManager("", WithStore(store), WithEnvironment("other-cluster"))
	sOther, _ := otherMgr.NewSession(Metadata{ProviderID: "p", ModelID: "m"})

	// Check sessions in cluster-test
	remaining, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	// MaxSessions is 2, so only 2 sessions should remain
	if len(remaining) != 2 {
		t.Fatalf("expected 2 sessions after eviction, got %d", len(remaining))
	}

	// The two newest sessions should be s4 and s3
	remainingIDs := map[string]bool{
		remaining[0].ID: true,
		remaining[1].ID: true,
	}
	if !remainingIDs[s4.ID] || !remainingIDs[s3.ID] {
		t.Errorf("expected sessions %s and %s to be retained, got %v", s4.ID, s3.ID, remainingIDs)
	}
	if remainingIDs[s1.ID] || remainingIDs[s2.ID] {
		t.Errorf("expected older sessions %s and %s to be evicted", s1.ID, s2.ID)
	}

	// Verify session in other environment was NOT evicted
	otherSessions, err := otherMgr.ListSessions()
	if err != nil {
		t.Fatalf("otherMgr.ListSessions failed: %v", err)
	}
	if len(otherSessions) != 1 || otherSessions[0].ID != sOther.ID {
		t.Errorf("other environment session was affected by eviction")
	}
}

func TestSessionEviction_TTL(t *testing.T) {
	store := newMemoryStore()
	policy := RetentionPolicy{
		TTL: 30 * time.Minute,
	}

	mgr, err := NewSessionManager("", WithStore(store), WithEnvironment("cluster-ttl"), WithRetentionPolicy(policy))
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	now := time.Now()

	// Old session: modified 2 hours ago
	oldSession := &api.Session{
		ID:           "old-session",
		Environment:  "cluster-ttl",
		CreatedAt:    now.Add(-2 * time.Hour),
		LastModified: now.Add(-2 * time.Hour),
	}
	if err := store.CreateSession(oldSession); err != nil {
		t.Fatalf("creating old session failed: %v", err)
	}

	// Fresh session: modified 5 minutes ago
	freshSession := &api.Session{
		ID:           "fresh-session",
		Environment:  "cluster-ttl",
		CreatedAt:    now.Add(-5 * time.Minute),
		LastModified: now.Add(-5 * time.Minute),
	}
	if err := store.CreateSession(freshSession); err != nil {
		t.Fatalf("creating fresh session failed: %v", err)
	}

	evictedIDs, err := mgr.EnforceRetention("cluster-ttl")
	if err != nil {
		t.Fatalf("EnforceRetention failed: %v", err)
	}

	if len(evictedIDs) != 1 || evictedIDs[0] != "old-session" {
		t.Errorf("expected evictedIDs to be ['old-session'], got %v", evictedIDs)
	}

	remaining, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "fresh-session" {
		t.Errorf("expected remaining session to be 'fresh-session', got %v", remaining)
	}
}

func TestOperationalMemory_StorageAndIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore := NewFileMemoryStore(tmpDir)

	mgrProd, _ := NewSessionManager("memory", WithMemoryStore(fileStore), WithEnvironment("prod"))
	mgrDev, _ := NewSessionManager("memory", WithMemoryStore(fileStore), WithEnvironment("dev"))

	entryProd := &MemoryEntry{
		Key:      "ingress-class",
		Content:  "Use internal-gke ingress class for internal microservices.",
		Category: "runbook",
	}

	if err := mgrProd.SaveMemory(entryProd); err != nil {
		t.Fatalf("SaveMemory in prod failed: %v", err)
	}

	// Prod should see it
	prodMemories, err := mgrProd.ListMemories("prod")
	if err != nil {
		t.Fatalf("ListMemories prod failed: %v", err)
	}
	if len(prodMemories) != 1 || prodMemories[0].Key != "ingress-class" {
		t.Errorf("expected 1 prod memory with key ingress-class, got %v", prodMemories)
	}

	// Dev should NOT see it (isolation)
	devMemories, err := mgrDev.ListMemories("dev")
	if err != nil {
		t.Fatalf("ListMemories dev failed: %v", err)
	}
	if len(devMemories) != 0 {
		t.Errorf("isolation violated: dev environment has %d memories, want 0", len(devMemories))
	}
}

func TestOperationalMemory_Eviction(t *testing.T) {
	memStore := NewInMemoryMemoryStore()
	policy := RetentionPolicy{
		MaxMemories: 2,
	}

	mgr, _ := NewSessionManager("memory", WithMemoryStore(memStore), WithEnvironment("env-evict"), WithRetentionPolicy(policy))

	_ = mgr.SaveMemory(&MemoryEntry{Key: "k1", Content: "c1"})
	time.Sleep(5 * time.Millisecond)
	_ = mgr.SaveMemory(&MemoryEntry{Key: "k2", Content: "c2"})
	time.Sleep(5 * time.Millisecond)
	_ = mgr.SaveMemory(&MemoryEntry{Key: "k3", Content: "c3"})

	memories, err := mgr.ListMemories("env-evict")
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}

	if len(memories) != 2 {
		t.Fatalf("expected 2 memories retained after eviction, got %d", len(memories))
	}

	keys := map[string]bool{memories[0].Key: true, memories[1].Key: true}
	if !keys["k2"] || !keys["k3"] {
		t.Errorf("expected newest keys k2 and k3 retained, got %v", keys)
	}
}

func TestOperationalMemory_SecurityCredentialRejection(t *testing.T) {
	tests := []struct {
		name      string
		entry     *MemoryEntry
		shouldErr bool
	}{
		{
			name: "clean operational runbook",
			entry: &MemoryEntry{
				Environment: "prod",
				Key:         "istio-preference",
				Content:     "Always verify virtualservice routing before applying gateway changes.",
			},
			shouldErr: false,
		},
		{
			name: "reject bearer token",
			entry: &MemoryEntry{
				Environment: "prod",
				Key:         "token",
				Content:     "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.xyz",
			},
			shouldErr: true,
		},
		{
			name: "reject password assignment",
			entry: &MemoryEntry{
				Environment: "prod",
				Key:         "creds",
				Content:     "db password = SuperSecretPassword123!",
			},
			shouldErr: true,
		},
		{
			name: "reject private key block",
			entry: &MemoryEntry{
				Environment: "prod",
				Key:         "tls-key",
				Content:     "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----",
			},
			shouldErr: true,
		},
		{
			name: "reject aws secret key",
			entry: &MemoryEntry{
				Environment: "prod",
				Key:         "aws",
				Content:     "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMemoryEntry(tt.entry)
			if tt.shouldErr && err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error for %s, got: %v", tt.name, err)
			}
		})
	}
}
