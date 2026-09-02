// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultLogFilePath_IsPerUser guards against #564: a shared, fixed
// /tmp/kubectl-ai.log path means the first user on a shared/edge node to run
// kubectl-ai creates the file with permissions that block every other user,
// who then fails outright with "unable to create log: ... permission
// denied" instead of getting their own log.
func TestDefaultLogFilePath_IsPerUser(t *testing.T) {
	path, err := defaultLogFilePath()
	if err != nil {
		t.Fatalf("defaultLogFilePath() returned error: %v", err)
	}

	if strings.HasPrefix(filepath.Clean(path), filepath.Clean(os.TempDir())) {
		t.Errorf("defaultLogFilePath() = %q, resolves under the shared system temp dir (os.TempDir()); "+
			"on a multi-user host this is the exact path collision from #564", path)
	}

	if filepath.Base(path) != "kubectl-ai.log" {
		t.Errorf("defaultLogFilePath() = %q, want a file named kubectl-ai.log", path)
	}

	// The parent directory must exist (and be creatable) so klog can open
	// the log file on first run, the same guarantee os.TempDir() gave for
	// free.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("defaultLogFilePath() parent directory not usable: %v", err)
	}
}

// TestDefaultLogFilePath_TwoUsersGetDifferentPaths simulates two different
// users (distinct HOME/USERPROFILE) and asserts they resolve to different
// log file paths, so one user's file permissions can never block another's.
func TestDefaultLogFilePath_TwoUsersGetDifferentPaths(t *testing.T) {
	// os.UserCacheDir() reads $HOME (via $XDG_CACHE_HOME) on Linux/macOS but
	// %LocalAppData% directly on Windows — set whichever this platform's
	// implementation actually consults, for each simulated user.
	setSimulatedUserCacheEnv := func(home string) {
		t.Setenv("HOME", home)
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("USERPROFILE", home)
		t.Setenv("LocalAppData", filepath.Join(home, "AppData", "Local"))
	}

	setSimulatedUserCacheEnv(t.TempDir())
	pathA, err := defaultLogFilePath()
	if err != nil {
		t.Fatalf("defaultLogFilePath() for user A returned error: %v", err)
	}

	setSimulatedUserCacheEnv(t.TempDir())
	pathB, err := defaultLogFilePath()
	if err != nil {
		t.Fatalf("defaultLogFilePath() for user B returned error: %v", err)
	}

	if pathA == pathB {
		t.Errorf("two different users resolved to the same log path %q — the #564 collision", pathA)
	}
}
