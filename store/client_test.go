package store

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// コミットオブジェクトが正しく取れるか
func TestClient_GetObject(t *testing.T) {
	testRepoPath := "./testdata/test_repo"

	// Create the test repository directory
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}
	// Cleanup the test repository directory after the test
	t.Cleanup(func() {
		if err := os.RemoveAll(testRepoPath); err != nil {
			t.Logf("Failed to remove test repo directory: %v", err)
		}
	})

	// Initialize a new Git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = testRepoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to git init: %v\nOutput: %s", err, string(output))
	}

	// Create a dummy file
	dummyFilePath := filepath.Join(testRepoPath, "dummy.txt")
	if err := os.WriteFile(dummyFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to write dummy file: %v", err)
	}

	// Stage the file
	cmd = exec.Command("git", "add", "dummy.txt")
	cmd.Dir = testRepoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to git add: %v\nOutput: %s", err, string(output))
	}

	// Create a commit
	cmd = exec.Command("git", "commit", "-m", "Test commit")
	cmd.Dir = testRepoPath
	// Need to set GIT_COMMITTER_NAME and GIT_COMMITTER_EMAIL for git commit to work
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to git commit: %v\nOutput: %s", err, string(output))
	}

	// Retrieve the hash of the commit
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = testRepoPath
	hashBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to git rev-parse HEAD: %v", err)
	}
	hashString := strings.TrimSpace(string(hashBytes))

	// Initialize client with the new test repository path
	client, err := NewClient(testRepoPath)
	if err != nil {
		t.Fatal(err)
	}

	hash, err := hex.DecodeString(hashString)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := client.GetObject(hash)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%s", obj.Type) != "commit" {
		t.Errorf("Expected object type 'commit', got '%s'", obj.Type)
	}
	t.Log(fmt.Sprintf("Successfully retrieved commit object with hash: %s, type: %s", hashString, obj.Type))
}
