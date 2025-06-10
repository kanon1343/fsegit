package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAddCommand(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "fsegit-add-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	// Use t.Cleanup to ensure os.RemoveAll is called at the end of the test
	t.Cleanup(func() { os.RemoveAll(tempDir) })


	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}
	// Use t.Cleanup for changing back directory, executed in LIFO order
	t.Cleanup(func() { os.Chdir(originalWd) })

	fsegitDir, objectsDir, _ := CreateTestRepo(t, tempDir) // Discard refsHeadsDir

	// Create sample files
	fileAContent := []byte("This is file A.")
	fileBContent := []byte("Content for file B.")
	if err := ioutil.WriteFile("fileA.txt", fileAContent, 0644); err != nil {
		t.Fatalf("Failed to write fileA.txt: %v", err)
	}
	if err := ioutil.WriteFile("fileB.txt", fileBContent, 0644); err != nil {
		t.Fatalf("Failed to write fileB.txt: %v", err)
	}

	// Setup cobra command for testing
	// We need a root command to attach our addCmd to for execution context
	testRootCmd := &cobra.Command{Use: "fsegit-test"}
	// The actual addCmd is a package-level variable in the 'cmd' package.
	// We add it to our test root command.
	testRootCmd.AddCommand(addCmd)

	// Reset any relevant states before execution
	// Pass testRootCmd and the specific command being tested (addCmd)
	ResetCommandStatesTest(t, testRootCmd, addCmd)


	// Execute the addCmd
	_, err = ExecuteCommandTest(testRootCmd, "add", "fileA.txt", "fileB.txt")
	if err != nil {
		t.Fatalf("addCmd execution failed: %v", err)
	}

	// Verify .fsegit/index
	indexFilePath := filepath.Join(fsegitDir, "index")
	indexData, err := ioutil.ReadFile(indexFilePath)
	if err != nil {
		t.Fatalf("Failed to read index file: %v", err)
	}

	indexEntries := strings.Split(strings.TrimSpace(string(indexData)), "\n")
	if len(indexEntries) != 2 {
		t.Fatalf("Expected 2 entries in index, got %d: %v", len(indexEntries), indexEntries)
	}

	expectedShaFileA := CalculateBlobSHATest(fileAContent)
	expectedShaFileB := CalculateBlobSHATest(fileBContent)
	foundFileA := false
	foundFileB := false

	for _, entry := range indexEntries {
		parts := strings.Fields(entry)
		if len(parts) != 2 {
			t.Errorf("Invalid index entry format: '%s'", entry)
			continue
		}
		filePath := parts[0]
		sha1Hash := parts[1]
		if filePath == "fileA.txt" {
			if sha1Hash != expectedShaFileA {
				t.Errorf("fileA.txt SHA mismatch: got %s, want %s", sha1Hash, expectedShaFileA)
			}
			foundFileA = true
		} else if filePath == "fileB.txt" {
			if sha1Hash != expectedShaFileB {
				t.Errorf("fileB.txt SHA mismatch: got %s, want %s", sha1Hash, expectedShaFileB)
			}
			foundFileB = true
		}
	}
	if !foundFileA {
		t.Errorf("fileA.txt not found in index")
	}
	if !foundFileB {
		t.Errorf("fileB.txt not found in index")
	}

	// Verify blob objects
	blobAData, err := ReadObjectTest(objectsDir, expectedShaFileA)
	if err != nil {
		t.Fatalf("Failed to read blob object for fileA.txt (SHA: %s): %v", expectedShaFileA, err)
	}
	expectedBlobAObjectContent := fmt.Sprintf("blob %d\x00%s", len(fileAContent), fileAContent)
	if string(blobAData) != expectedBlobAObjectContent {
		t.Errorf("fileA.txt blob content mismatch: got '%s', want '%s'", string(blobAData), expectedBlobAObjectContent)
	}

	blobBData, err := ReadObjectTest(objectsDir, expectedShaFileB)
	if err != nil {
		t.Fatalf("Failed to read blob object for fileB.txt (SHA: %s): %v", expectedShaFileB, err)
	}
	expectedBlobBObjectContent := fmt.Sprintf("blob %d\x00%s", len(fileBContent), fileBContent)
	if string(blobBData) != expectedBlobBObjectContent {
		t.Errorf("fileB.txt blob content mismatch: got '%s', want '%s'", string(blobBData), expectedBlobBObjectContent)
	}

	t.Log("Add command standalone test verification complete.")
}
