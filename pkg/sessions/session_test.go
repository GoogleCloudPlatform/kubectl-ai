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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionPersistence tests the basic save and load functionality
func TestSessionPersistence(t *testing.T) {
	// Create a temporary directory for test sessions
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a session manager
	manager := &SessionManager{BasePath: tempDir}

	// Create metadata for the session
	meta := Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	}

	// Create a new session
	session, err := manager.NewSession(meta)
	require.NoError(t, err)

	// Add some test messages
	testMessage := &api.Message{
		ID:        uuid.New().String(),
		Source:    api.MessageSourceUser,
		Type:      api.MessageTypeText,
		Payload:   "Hello, how can I help?",
		Timestamp: time.Now(),
	}
	err = session.AddChatMessage(testMessage)
	require.NoError(t, err)

	// Load the session and verify its contents
	loadedSession, err := manager.FindSessionByID(session.ID)
	require.NoError(t, err)

	messages := loadedSession.ChatMessages()
	require.Equal(t, 1, len(messages))
	assert.Equal(t, testMessage.Payload, messages[0].Payload)

	// Verify metadata
	loadedMeta, err := loadedSession.LoadMetadata()
	require.NoError(t, err)
	assert.Equal(t, meta.ProviderID, loadedMeta.ProviderID)
	assert.Equal(t, meta.ModelID, loadedMeta.ModelID)
}

// TestCreateNewSession tests the creation of a new session
func TestCreateNewSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	meta := Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	}

	session, err := manager.NewSession(meta)
	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.NotEmpty(t, session.Path)

	// Verify history file is created after adding a message:
	testMessage := &api.Message{
		ID:        uuid.New().String(),
		Source:    api.MessageSourceUser,
		Type:      api.MessageTypeText,
		Payload:   "Test message",
		Timestamp: time.Now(),
	}
	err = session.AddChatMessage(testMessage)
	require.NoError(t, err)
	assert.FileExists(t, session.HistoryPath())

	// Verify session directory and files exist
	assert.DirExists(t, session.Path)
	assert.FileExists(t, session.MetadataPath())
	assert.FileExists(t, session.HistoryPath())

	// Verify metadata
	loadedMeta, err := session.LoadMetadata()
	require.NoError(t, err)
	assert.Equal(t, meta.ProviderID, loadedMeta.ProviderID)
	assert.Equal(t, meta.ModelID, loadedMeta.ModelID)
	assert.False(t, loadedMeta.CreatedAt.IsZero())
	assert.False(t, loadedMeta.LastAccessed.IsZero())
}

// TestDeleteSession tests session deletion
func TestDeleteSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Delete the session
	err = manager.DeleteSession(session.ID)
	require.NoError(t, err)

	// Verify session directory is gone
	_, err = os.Stat(session.Path)
	assert.True(t, os.IsNotExist(err))

	// Verify session can't be found
	_, err = manager.FindSessionByID(session.ID)
	assert.Error(t, err)
}

// TestListSessions tests listing all available sessions
func TestListSessions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create multiple sessions
	for i := 0; i < 3; i++ {
		_, err := manager.NewSession(Metadata{
			ProviderID: "test-provider",
			ModelID:    "test-model",
		})
		require.NoError(t, err)
	}

	// List sessions
	sessions, err := manager.ListSessions()
	require.NoError(t, err)
	assert.Equal(t, 3, len(sessions))

	// Verify sessions are sorted by ID (newest first)
	for i := 1; i < len(sessions); i++ {
		assert.True(t, sessions[i-1].ID > sessions[i].ID)
	}
}

// TestCorruptedMetadata tests handling of corrupted metadata
func TestCorruptedMetadata(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Corrupt the metadata file
	err = os.WriteFile(session.MetadataPath(), []byte("corrupted yaml"), 0644)
	require.NoError(t, err)

	// Attempt to load metadata
	_, err = session.LoadMetadata()
	assert.Error(t, err)
}

// TestCorruptedHistory tests handling of corrupted history file
func TestCorruptedHistory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Add a valid message
	err = session.AddChatMessage(&api.Message{
		ID:        uuid.New().String(),
		Source:    api.MessageSourceUser,
		Type:      api.MessageTypeText,
		Payload:   "Valid message",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Append corrupted JSON to history file
	f, err := os.OpenFile(session.HistoryPath(), os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("corrupted json\n")
	require.NoError(t, err)
	f.Close()

	// Verify we can still read valid messages
	messages := session.ChatMessages()
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "Valid message", messages[0].Payload)
}

// TestConcurrentAccess tests concurrent access to a session
func TestConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Test concurrent reads and writes
	done := make(chan bool)
	messageCount := 100

	for i := 0; i < messageCount; i++ {
		go func(i int) {
			msg := &api.Message{
				ID:        uuid.New().String(),
				Source:    api.MessageSourceUser,
				Type:      api.MessageTypeText,
				Payload:   "Concurrent message",
				Timestamp: time.Now(),
			}
			err := session.AddChatMessage(msg)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < messageCount; i++ {
		<-done
	}

	// Verify all messages were written
	messages := session.ChatMessages()
	assert.Equal(t, messageCount, len(messages))
}

// TestClearMessages tests clearing all messages from a session
func TestClearMessages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Add some messages
	for i := 0; i < 3; i++ {
		err = session.AddChatMessage(&api.Message{
			ID:        uuid.New().String(),
			Source:    api.MessageSourceUser,
			Type:      api.MessageTypeText,
			Payload:   "Test message",
			Timestamp: time.Now(),
		})
		require.NoError(t, err)
	}

	// Verify messages were added
	assert.Equal(t, 3, len(session.ChatMessages()))

	// Clear messages
	err = session.ClearChatMessages()
	require.NoError(t, err)

	// Verify messages were cleared
	assert.Empty(t, session.ChatMessages())
}

// TestGetLatestSession tests getting the most recent session
func TestGetLatestSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create multiple sessions
	var lastSession *Session
	for i := 0; i < 3; i++ {
		lastSession, err = manager.NewSession(Metadata{
			ProviderID: "test-provider",
			ModelID:    "test-model",
		})
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// List sessions for correct order
	manager.ListSessions()

	// Get latest session
	latest, err := manager.GetLatestSession()
	require.NoError(t, err)
	assert.Equal(t, lastSession.ID, latest.ID)
}

// TestUpdateLastAccessed tests updating the last accessed timestamp
func TestUpdateLastAccessed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &SessionManager{BasePath: tempDir}

	// Create a session
	session, err := manager.NewSession(Metadata{
		ProviderID: "test-provider",
		ModelID:    "test-model",
	})
	require.NoError(t, err)

	// Get initial last accessed time
	meta, err := session.LoadMetadata()
	require.NoError(t, err)
	initialAccess := meta.LastAccessed

	time.Sleep(time.Millisecond) // Ensure different timestamp

	// Update last accessed
	err = session.UpdateLastAccessed()
	require.NoError(t, err)

	// Verify last accessed was updated
	meta, err = session.LoadMetadata()
	require.NoError(t, err)
	assert.True(t, meta.LastAccessed.After(initialAccess))
}
