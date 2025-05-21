package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanon1343/fsegit/store" // For ReadIndex
	// No direct object interaction needed for ls-files tests if we rely on addCmd
	// to set up the index correctly.
)

// TestLsFilesEmptyIndex tests ls-files on an empty index.
func TestLsFilesEmptyIndex(t *testing.T) {
	repoRoot, _, cleanup := setupTestRepoLsFiles(t) // Using specific helper
	defer cleanup()

	originalWD, _ := os.Getwd()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}
	defer os.Chdir(originalWD)

	var out bytes.Buffer
	lsCmdLocal := *lsFilesCmd // Make a copy
	lsCmdLocal.SetOut(&out)    // Capture stdout
	lsCmdLocal.SetErr(&out)    // Capture stderr for error messages too
	lsCmdLocal.SetArgs([]string{}) // No args

	// Reset flags for ls-files if they are persistent or global
	showStage = false // Resetting global flag from lsFiles.go

	if err := lsCmdLocal.Execute(); err != nil {
		t.Fatalf("ls-files command execution failed: %v\nOutput:\n%s", err, out.String())
	}

	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("Expected no output for empty index, got:\n%s", out.String())
	}
}

// TestLsFilesWithFiles tests ls-files with items in the index.
func TestLsFilesWithFiles(t *testing.T) {
	repoRoot, dotGitPath, cleanup := setupTestRepoLsFiles(t)
	defer cleanup()

	originalWD, _ := os.Getwd()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}
	defer os.Chdir(originalWD)

	// Add some files
	file1Name := "file1.txt"
	file1Content := "content1"
	createFileLsFiles(t, repoRoot, file1Name, file1Content)

	file2Name := "dir/file2.txt" // Path with directory
	file2Content := "content2"
	createFileLsFiles(t, repoRoot, file2Name, file2Content)
	
	// Use a local copy of addCmd to add files to the index
	addCmdLocal := *addCmd
	addCmdLocal.SetArgs([]string{file1Name, file2Name})
	// Suppress output from addCmd during test setup
	addCmdLocal.SetOut(ioutil.Discard)
	addCmdLocal.SetErr(ioutil.Discard)
	if err := addCmdLocal.Execute(); err != nil {
		t.Fatalf("add command execution failed during setup: %v", err)
	}
	
	// Test ls-files (no stage)
	var out bytes.Buffer
	lsCmdLocal := *lsFilesCmd
	lsCmdLocal.SetOut(&out)
	lsCmdLocal.SetErr(&out)
	lsCmdLocal.SetArgs([]string{})
	
	showStage = false // Reset global flag from lsFiles.go
	
	if err := lsCmdLocal.Execute(); err != nil {
		t.Fatalf("ls-files (no stage) execution failed: %v\nOutput:\n%s", err, out.String())
	}

	// Read the index to get the canonical sorted order of paths for assertion.
	idx, err := store.ReadIndex(dotGitPath)
	if err != nil {
		t.Fatalf("Failed to read index for verification: %v", err)
	}
	
	var expectedPaths []string
	for _, entry := range idx.Entries {
		expectedPaths = append(expectedPaths, entry.PathName)
	}
	// Output from ls-files should be sorted like the index entries (by PathName).
	expectedOutputSorted := strings.Join(expectedPaths, "\n")


	if strings.TrimSpace(out.String()) != strings.TrimSpace(expectedOutputSorted) {
		t.Errorf("Expected ls-files output:\n---\n%s\n---\nGot:\n---\n%s\n---", expectedOutputSorted, out.String())
	}

	// Test ls-files --stage
	out.Reset()
	lsCmdLocal.SetArgs([]string{"--stage"}) // Set args for --stage
	showStage = true // Set global flag for --stage (from lsFiles.go)
	
	if err := lsCmdLocal.Execute(); err != nil {
		t.Fatalf("ls-files --stage execution failed: %v\nOutput:\n%s", err, out.String())
	}

	var expectedStageOutputBuilder strings.Builder
	// Entries should already be sorted as they come from idx.Entries which is sorted by ReadIndex or by AddEntry + WriteIndex path
	for _, entry := range idx.Entries {
		stageNum := (entry.Flags >> 12) & 0x3
		expectedStageOutputBuilder.WriteString(
			fmt.Sprintf("%06o %s %d	%s\n", entry.Mode, entry.Hash.String(), stageNum, entry.PathName),
		)
	}
	// Trim trailing newline from builder if present, and from actual output for consistent comparison
	expectedStageOutput := strings.TrimSpace(expectedStageOutputBuilder.String())

	if strings.TrimSpace(out.String()) != expectedStageOutput {
		t.Errorf("Expected ls-files --stage output:\n---\n%s\n---\nGot:\n---\n%s\n---", expectedStageOutput, out.String())
	}
}


// Helper functions (copied from other test files for now)
// Renamed to avoid potential conflicts if these files were ever in the same package directly
// without being part of a 'cmd_test' package which Go handles separately.
func setupTestRepoLsFiles(t *testing.T) (string, string, func()) {
	t.Helper()
	tmpDir, err := ioutil.TempDir("", "fsegit_test_lsfiles_")
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
	// Create an empty index file, as `add` command will try to read it.
	// If `store.ReadIndex` handles non-existent gracefully, this isn't strictly needed
	// but it's safer for tests depending on `addCmd`.
	if _, err := os.Create(filepath.Join(dotGitPath, "index")); err != nil {
        os.RemoveAll(tmpDir)
        t.Fatalf("Failed to create empty index file: %v", err)
    }

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	return tmpDir, dotGitPath, cleanup
}

func createFileLsFiles(t *testing.T, repoRoot, filePath, content string) string {
	t.Helper()
	fullPath := filepath.Join(repoRoot, filePath)
	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		t.Fatalf("Failed to create parent dirs for %s (tried %s): %v", filePath, parentDir, err)
	}
	if err := ioutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", filePath, err)
	}
	return fullPath
}
```
