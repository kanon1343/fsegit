package cmd

import (
	"bytes"
	// "compress/zlib" // Not directly used in this test snippet, but might be by GetObject if it decompresses
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time" // Not directly used in this test snippet but often useful

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/kanon1343/fsegit/util"
)

// setupTestRepo creates a temporary directory, initializes a .git structure,
// and returns the path to the temporary repo root and the .git dir.
// It also returns a cleanup function to be called with defer.
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
	// Create HEAD file pointing to master
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

// createFile is a helper to create a file with content in the test repo.
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

// TestAddSingleFile tests adding a single new file.
func TestAddSingleFile(t *testing.T) {
	repoRoot, dotGitPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Change working directory to repoRoot for the add command to work as expected
	// as it uses relative paths like "." to find .git
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Failed to change working directory to %s: %v", repoRoot, err)
	}
	defer os.Chdir(originalWD) // Change back

	fileName := "test.txt"
	fileContent := "Hello, Git!"
	createFile(t, repoRoot, fileName, fileContent)

	// Execute the add command's Run function directly
	// Need to reset rootCmd for each test or manage flags carefully if using Execute()
	addCmdLocal := *addCmd // Make a copy to avoid race conditions if tests run in parallel
	addCmdLocal.SetArgs([]string{fileName})
	
	// Capture stdout/stderr to check for errors if needed, though addCmd logs to log.Fatal on critical errors
	// For now, we check results by inspecting index and objects.
	// Using Execute() to run the command, which handles cobra's lifecycle including flag parsing.
	// Redirect cobra command output to avoid polluting test output
	var cmdOutput bytes.Buffer
	rootCmd.SetOut(&cmdOutput) // Assuming addCmd is a child of a global rootCmd
	rootCmd.SetErr(&cmdOutput) // If not, set on addCmdLocal directly: addCmdLocal.SetOut(&cmdOutput)
	
	if err := addCmdLocal.Execute(); err != nil { 
		t.Fatalf("add command execution failed: %v\nOutput:\n%s", err, cmdOutput.String())
	}


	// 1. Verify index
	idx, err := store.ReadIndex(dotGitPath)
	if err != nil {
		t.Fatalf("Failed to read index after add: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("Expected 1 entry in index, got %d", len(idx.Entries))
	}
	entry := idx.Entries[0]
	// PathName in index should be relative and use slashes
	expectedPathName := strings.ReplaceAll(fileName, string(filepath.Separator), "/")
	if entry.PathName != expectedPathName {
		t.Errorf("Expected path name '%s', got '%s'", expectedPathName, entry.PathName)
	}
	expectedMode := uint32(FileModeRegular) // Defined in add.go
	if entry.Mode != expectedMode {
		t.Errorf("Expected mode %06o, got %06o", expectedMode, entry.Mode)
	}
	if entry.Size != uint32(len(fileContent)) {
		t.Errorf("Expected size %d, got %d", len(fileContent), entry.Size)
	}

	// 2. Verify blob object
	storeClient, err := store.NewClient(repoRoot) // repoRoot should be correct for client
	if err != nil {
		t.Fatalf("Failed to create store client for verification: %v", err)
	}
	
	blobObj, err := storeClient.GetObject(entry.Hash)
	if err != nil {
		t.Fatalf("Failed to get blob object %s from store: %v", entry.Hash.String(), err)
	}
	if blobObj.Type != object.BlobObject { // object.BlobObject should be from the imported object package
		t.Errorf("Expected object type 'blob', got '%s'", blobObj.Type)
	}
	if string(blobObj.Data) != fileContent {
		t.Errorf("Expected blob data '%s', got '%s'", fileContent, string(blobObj.Data))
	}

	// Verify the hash calculation consistency (optional, NewBlob and GetObject should ensure this)
	expectedBlob := object.NewBlob([]byte(fileContent)) // object.NewBlob
	if !bytes.Equal(expectedBlob.Hash, entry.Hash) {
		t.Errorf("Blob hash mismatch: index has %s, calculated %s", entry.Hash.String(), expectedBlob.Hash.String())
	}
}

// TODO: TestAddMultipleFiles
// TODO: TestAddModifiedFile
// TODO: TestAddFileInSubdirectory
// TODO: TestAddNonExistentFile
