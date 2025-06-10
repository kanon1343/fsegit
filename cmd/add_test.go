package cmd

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Helper function to execute cobra commands and capture output/error
// Copied from add_commit_test.go
func executeCommandTest(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	// For 'add' and 'commit' commands, they are added to the rootCmd in their init.
	// If rootCmd is created anew here, it needs its subcommands re-added.
	// However, the actual addCmd and commitCmd are global vars in the package.
	// We need to ensure flags are reset if they are persistent or package-level.
	// For 'add', there are no flags. For 'commit', 'commitMessage' is a package var.

	err := root.Execute()
	return strings.TrimSpace(buf.String()), err
}

// Helper function to calculate blob SHA (for verification)
// Copied from add_commit_test.go
func calculateBlobSHATest(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	data := append([]byte(header), content...)
	hash := sha1.Sum(data)
	return fmt.Sprintf("%x", hash)
}

// Helper function to read and decompress an object file
// Renamed to avoid conflict if these were in the same package and not _test package
// Copied from add_commit_test.go
func readObjectTest(objectDir, sha1Str string) ([]byte, error) {
	path := filepath.Join(objectDir, sha1Str[:2], sha1Str[2:])
	compressedData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read object file %s: %w", path, err)
	}

	reader, err := zlib.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader for %s: %w", sha1Str, err)
	}
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress object %s: %w", sha1Str, err)
	}
	return data, nil
}

// Resets global state for commands, particularly for flags or package-level vars.
// Copied and adapted from add_commit_test.go
func resetCommandStatesTest(t *testing.T) {
	// For addCmd, there's no global flag state to reset that cobra doesn't handle per-run.
	// For commitCmd (if it were used here), commitMessage is a global var.
	// commitMessage = "" // Example if commitCmd was involved

	// Re-initialize root command and its children for a clean state for THIS test run.
	// This helps if subcommands are added to a global rootCmd instance.
	// However, our actual commands (addCmd, commitCmd) are package-level variables.
	// The executeCommandTest will use the passed rootCmd.
	// The main thing is to ensure any package-level flags used by commands are reset.
	// addCmd doesn't have such flags.
}

// Helper to create a basic repo structure for tests
func createTestRepo(t *testing.T, tempDir string) (fsegitDir, objectsDir string) {
	t.Helper()
	fsegitDir = filepath.Join(tempDir, ".fsegit")
	objectsDir = filepath.Join(fsegitDir, "objects")
	refsHeadsDir := filepath.Join(fsegitDir, "refs", "heads")

	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", objectsDir, err)
	}
	if err := os.MkdirAll(refsHeadsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", refsHeadsDir, err)
	}
	return fsegitDir, objectsDir
}


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

	fsegitDir, objectsDir := createTestRepo(t, tempDir)

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

	// Reset any relevant states before execution (though addCmd has none currently)
	resetCommandStatesTest(t) // Currently a no-op for addCmd

	// Execute the addCmd
	_, err = executeCommandTest(testRootCmd, "add", "fileA.txt", "fileB.txt")
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

	expectedShaFileA := calculateBlobSHATest(fileAContent)
	expectedShaFileB := calculateBlobSHATest(fileBContent)
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
	blobAData, err := readObjectTest(objectsDir, expectedShaFileA)
	if err != nil {
		t.Fatalf("Failed to read blob object for fileA.txt (SHA: %s): %v", expectedShaFileA, err)
	}
	expectedBlobAObjectContent := fmt.Sprintf("blob %d\x00%s", len(fileAContent), fileAContent)
	if string(blobAData) != expectedBlobAObjectContent {
		t.Errorf("fileA.txt blob content mismatch: got '%s', want '%s'", string(blobAData), expectedBlobAObjectContent)
	}

	blobBData, err := readObjectTest(objectsDir, expectedShaFileB)
	if err != nil {
		t.Fatalf("Failed to read blob object for fileB.txt (SHA: %s): %v", expectedShaFileB, err)
	}
	expectedBlobBObjectContent := fmt.Sprintf("blob %d\x00%s", len(fileBContent), fileBContent)
	if string(blobBData) != expectedBlobBObjectContent {
		t.Errorf("fileB.txt blob content mismatch: got '%s', want '%s'", string(blobBData), expectedBlobBObjectContent)
	}

	t.Log("Add command standalone test verification complete.")
}
