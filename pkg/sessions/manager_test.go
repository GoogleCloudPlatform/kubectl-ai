// Copyright 2026 Google LLC
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
	"testing"
	"time"
)

// TestSessionManager_Lifecycle verifies session creation, retrieval, listing, deletion,
// and latest session resolution using filesystem backend.
func TestSessionManager_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	fsStore := newFilesystemStore(tempDir)
	sm := &SessionManager{store: fsStore}

	// 1. Initially empty
	sessions, err := sm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions initially, got %d", len(sessions))
	}

	latest, err := sm.GetLatestSession()
	if err != nil {
		t.Fatalf("GetLatestSession failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil latest session, got %+v", latest)
	}

	// 2. Create first session
	meta1 := Metadata{
		ProviderID: "llamacpp",
		ModelID:    "llama-3.2-3b",
	}
	session1, err := sm.NewSession(meta1)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	if session1 == nil || session1.ID == "" {
		t.Fatalf("expected valid session with non-empty ID")
	}
	if session1.ProviderID != "llamacpp" || session1.ModelID != "llama-3.2-3b" {
		t.Errorf("unexpected session metadata: %+v", session1)
	}

	// 3. Find first session by ID
	found, err := sm.FindSessionByID(session1.ID)
	if err != nil {
		t.Fatalf("FindSessionByID failed: %v", err)
	}
	if found.ID != session1.ID {
		t.Errorf("expected session ID %q, got %q", session1.ID, found.ID)
	}

	// Wait slightly to ensure distinguishable timestamp
	time.Sleep(10 * time.Millisecond)

	// 4. Create second session
	meta2 := Metadata{
		ProviderID: "llamacpp",
		ModelID:    "llama-3.3-70b",
	}
	session2, err := sm.NewSession(meta2)
	if err != nil {
		t.Fatalf("NewSession 2 failed: %v", err)
	}

	// 5. List sessions
	sessions, err = sm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// 6. Get latest session
	latest, err = sm.GetLatestSession()
	if err != nil {
		t.Fatalf("GetLatestSession failed: %v", err)
	}
	if latest == nil || latest.ID != session2.ID {
		t.Errorf("expected latest session to be %q, got %+v", session2.ID, latest)
	}

	// 7. Delete first session
	if err := sm.DeleteSession(session1.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify session1 is gone
	_, err = sm.FindSessionByID(session1.ID)
	if err == nil {
		t.Fatalf("expected error finding deleted session, got nil")
	}

	sessions, err = sm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != session2.ID {
		t.Fatalf("expected only session2 to remain, got %v", sessions)
	}
}

// TestSessionManager_EdgeCases verifies edge case handling like non-existent session
// and corrupted session metadata.
func TestSessionManager_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	fsStore := newFilesystemStore(tempDir)
	sm := &SessionManager{store: fsStore}

	t.Run("non-existent session returns error", func(t *testing.T) {
		session, err := sm.FindSessionByID("non-existent-session-id")
		if err == nil {
			t.Errorf("expected error when fetching non-existent session, got session: %+v", session)
		}
	})

	t.Run("delete non-existent session does not panic", func(t *testing.T) {
		// Deleting a non-existent session directory is a no-op or returns error
		_ = sm.DeleteSession("non-existent-session-id")
	})

	t.Run("corrupted metadata file returns error", func(t *testing.T) {
		corruptedDir := filepath.Join(tempDir, "corrupted-session")
		if err := os.MkdirAll(corruptedDir, 0o755); err != nil {
			t.Fatalf("failed to create corrupted session dir: %v", err)
		}
		// Write invalid YAML into metadata.yaml
		if err := os.WriteFile(filepath.Join(corruptedDir, "metadata.yaml"), []byte("invalid: yaml: :::"), 0o644); err != nil {
			t.Fatalf("failed to write corrupted metadata: %v", err)
		}

		_, err := sm.FindSessionByID("corrupted-session")
		if err == nil {
			t.Errorf("expected error for corrupted metadata, got nil")
		}
	})
}
