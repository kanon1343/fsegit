package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	// "time" // For more precise author/committer time checks if needed later

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/kanon1343/fsegit/sha" // For sha.FromString
	// util.FindGitRoot is not directly used by tests if we always operate from repoRoot
)

// TestCommitInitial creates the first commit in a new repository.
func TestCommitInitial(t *testing.T) {
	repoRoot, dotGitPath, cleanup := setupTestRepo(t) // Using helper from add_test.go
	defer cleanup()

	originalWD, _ := os.Getwd()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Failed to change working directory to %s: %v", repoRoot, err)
	}
	defer os.Chdir(originalWD)

	// 1. Add a file to the index
	fileName := "initial.txt"
	fileContent := "Initial commit content."
	createFile(t, repoRoot, fileName, fileContent) // Using helper from add_test.go

	addCmdLocal := *addCmd 
	addCmdLocal.SetArgs([]string{fileName})
	// Suppress output from addCmd
	addCmdLocal.SetOut(ioutil.Discard)
	addCmdLocal.SetErr(ioutil.Discard)
	if err := addCmdLocal.Execute(); err != nil {
		t.Fatalf("add command execution failed: %v", err)
	}

	// 2. Execute the commit command
	commitMsg := "Initial commit"
	commitCmdLocal := *commitCmd 
	
	originalCommitMessage := commitMessage 
	commitMessage = commitMsg 
	defer func() { commitMessage = originalCommitMessage }() 

	commitCmdLocal.SetArgs([]string{"-m", commitMsg}) 
	
	// Suppress output from commitCmd
	commitCmdLocal.SetOut(ioutil.Discard)
	commitCmdLocal.SetErr(ioutil.Discard)
	
	if err := commitCmdLocal.Execute(); err != nil {
		t.Fatalf("commit command execution failed: %v", err)
	}

	// 3. Verify the commit
	storeClient, err := store.NewClient(repoRoot)
	if err != nil {
		t.Fatalf("Failed to create store client: %v", err)
	}

	// 3a. Verify HEAD and branch ref (e.g., refs/heads/master)
	headPath := filepath.Join(dotGitPath, "HEAD")
	headContentBytes, err := ioutil.ReadFile(headPath)
	if err != nil {
		t.Fatalf("Failed to read HEAD: %v", err)
	}
	if !strings.HasPrefix(string(headContentBytes), "ref: refs/heads/") {
		t.Fatalf("HEAD does not point to a branch ref: %s", string(headContentBytes))
	}
	// Assuming default branch is master for this test, as setupTestRepo creates HEAD pointing to master
	branchRefPath := filepath.Join(dotGitPath, "refs", "heads", "master") 
	newCommitSHABytes, err := ioutil.ReadFile(branchRefPath)
	if err != nil {
		t.Fatalf("Failed to read branch ref %s: %v", branchRefPath, err)
	}
	newCommitSHAStr := strings.TrimSpace(string(newCommitSHABytes))
	if len(newCommitSHAStr) != 40 {
		t.Fatalf("Invalid commit SHA length in ref %s: %s", branchRefPath, newCommitSHAStr)
	}
	commitSHA, err := sha.FromString(newCommitSHAStr)
	if err != nil {
		t.Fatalf("Failed to parse commit SHA from ref %s: %v", branchRefPath, err)
	}

	// 3b. Retrieve and verify the commit object
	commitObjRaw, err := storeClient.GetObject(commitSHA)
	if err != nil {
		t.Fatalf("Failed to get commit object %s: %v", commitSHA.String(), err)
	}
	if commitObjRaw.Type != object.CommitObject {
		t.Fatalf("Expected commit object, got %s", commitObjRaw.Type)
	}
	
	parsedCommit, err := object.NewCommit(commitObjRaw) // NewCommit parses the object data
	if err != nil {
		t.Fatalf("Failed to parse commit object %s: %v", commitSHA.String(), err)
	}

	if parsedCommit.Message != commitMsg {
		t.Errorf("Expected commit message '%s', got '%s'", commitMsg, parsedCommit.Message)
	}
	if len(parsedCommit.Parents) != 0 {
		t.Errorf("Expected 0 parents for initial commit, got %d", len(parsedCommit.Parents))
	}
	// TODO: Verify author/committer (once not hardcoded in commit.go)

	// 3c. Retrieve and verify the root tree object
	rootTreeSHA := parsedCommit.Tree
	treeObjRaw, err := storeClient.GetObject(rootTreeSHA)
	if err != nil {
		t.Fatalf("Failed to get root tree object %s: %v", rootTreeSHA.String(), err)
	}
	if treeObjRaw.Type != object.TreeObject {
		t.Fatalf("Expected tree object for root, got %s", treeObjRaw.Type)
	}
	parsedTree, err := object.ParseTree(treeObjRaw.Data)
	if err != nil {
		t.Fatalf("Failed to parse root tree object %s: %v", rootTreeSHA.String(), err)
	}
	if len(parsedTree.Entries) != 1 {
		t.Fatalf("Expected 1 entry in root tree, got %d", len(parsedTree.Entries))
	}
	treeEntry := parsedTree.Entries[0]
	if treeEntry.Name != fileName {
		t.Errorf("Expected tree entry name '%s', got '%s'", fileName, treeEntry.Name)
	}
	
	// 3d. Verify the blob object referenced by the tree entry
	blobSHA := treeEntry.Hash
	blobObjRaw, err := storeClient.GetObject(blobSHA)
	if err != nil {
		t.Fatalf("Failed to get blob object %s from tree: %v", blobSHA.String(), err)
	}
	if string(blobObjRaw.Data) != fileContent {
		t.Errorf("Expected blob content '%s', got '%s'", fileContent, string(blobObjRaw.Data))
	}
}

// TODO: TestCommitSubsequent (with a parent)
// TODO: TestCommitWithSubdirectories

// Helpers from add_test.go (could be moved to a common test_util.go)
// For now, tests in different files can't directly share non-exported functions.
// If these helpers are needed here, they should be copied or made part of an importable package.
// For this subtask, assume setupTestRepo and createFile are available (e.g. copied or worker handles it)

func setupTestRepo(t *testing.T) (string, string, func()) {
	t.Helper()
	tmpDir, err := ioutil.TempDir("", "fsegit_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dotGitPath := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(filepath.Join(dotGitPath, "objects"), 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create .git/objects dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dotGitPath, "refs", "heads"), 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create .git/refs/heads dir: %v", err)
	}
	headFilePath := filepath.Join(dotGitPath, "HEAD")
	if err := ioutil.WriteFile(headFilePath, []byte("ref: refs/heads/master\n"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to write HEAD file: %v", err)
	}
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	return tmpDir, dotGitPath, cleanup
}

func createFile(t *testing.T, repoRoot, filePath, content string) string {
	t.Helper()
	fullPath := filepath.Join(repoRoot, filePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("Failed to create parent dirs for %s: %v", filePath, err)
	}
	if err := ioutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", filePath, err)
	}
	return fullPath
}

```
