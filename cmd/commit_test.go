package cmd

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- Copied Helper Functions (from add_test.go / add_commit_test.go) ---

// executeCommandTest executes cobra commands for testing.
func executeCommandTest(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return strings.TrimSpace(buf.String()), err
}

// calculateBlobSHATest calculates SHA1 for blob content.
func calculateBlobSHATest(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	data := append([]byte(header), content...)
	hash := sha1.Sum(data)
	return fmt.Sprintf("%x", hash)
}

// storeObjectTest compresses and stores data, mimicking storeObject from commit.go
func storeObjectTest(t *testing.T, objectsDir string, sha1Str string, data []byte) {
	t.Helper()
	objectSubDir := filepath.Join(objectsDir, sha1Str[:2])
	objectPath := filepath.Join(objectSubDir, sha1Str[2:])

	if err := os.MkdirAll(objectSubDir, 0755); err != nil {
		t.Fatalf("Failed to create object subdir %s: %v", objectSubDir, err)
	}

	objectFile, err := os.Create(objectPath)
	if err != nil {
		t.Fatalf("Failed to create object file %s: %v", objectPath, err)
	}
	defer objectFile.Close()

	zlibWriter := zlib.NewWriter(objectFile)
	if _, err := zlibWriter.Write(data); err != nil {
		t.Fatalf("Failed to write compressed data to object file %s: %v", objectPath, err)
	}
	if err := zlibWriter.Close(); err != nil {
		t.Fatalf("Failed to close zlib writer for object file %s: %v", objectPath, err)
	}
}


// readObjectTest reads and decompresses an object file.
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

// resetCommitCmdState resets state for commitCmd, specifically the message flag.
func resetCommitCmdState() {
	commitMessage = "" // This is the package-level variable for the -m flag in commit.go
}

// createTestRepo sets up a basic .fsegit structure.
func createTestRepo(t *testing.T, tempDir string) (fsegitDir, objectsDir, refsHeadsDir string) {
	t.Helper()
	fsegitDir = filepath.Join(tempDir, ".fsegit")
	objectsDir = filepath.Join(fsegitDir, "objects")
	refsHeadsDir = filepath.Join(fsegitDir, "refs", "heads")

	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", objectsDir, err)
	}
	if err := os.MkdirAll(refsHeadsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", refsHeadsDir, err)
	}
	return fsegitDir, objectsDir, refsHeadsDir
}

// --- TestCommitCommand ---

func TestCommitCommand(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "fsegit-commit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalWd) })

	fsegitDir, objectsDir, refsHeadsDir := createTestRepo(t, tempDir)

	// 1. Prepare for commit (simulate 'fsegit add')
	fileCContent := []byte("This is file C for commit test.")
	filePathC := "fileC.txt"
	if err := ioutil.WriteFile(filePathC, fileCContent, 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", filePathC, err)
	}

	// Manually create blob object for fileC.txt
	blobCSha1 := calculateBlobSHATest(fileCContent)
	blobCHeader := fmt.Sprintf("blob %d\x00", len(fileCContent))
	blobCData := append([]byte(blobCHeader), fileCContent...)
	storeObjectTest(t, objectsDir, blobCSha1, blobCData)

	// Manually create .fsegit/index
	indexFilePath := filepath.Join(fsegitDir, "index")
	indexContent := fmt.Sprintf("%s %s\n", filePathC, blobCSha1)
	if err := ioutil.WriteFile(indexFilePath, []byte(indexContent), 0644); err != nil {
		t.Fatalf("Failed to write index file: %v", err)
	}

	// 2. Execute the commitCmd
	testCommitMsg := "Test commit C"

	// Setup cobra command for testing
	// Important: commitCmd uses a package-level variable `commitMessage` for the -m flag.
	// We must ensure this is reset.
	resetCommitCmdState()

	testRootCmd := &cobra.Command{Use: "fsegit-test"}
	// commitCmd is a global var in the 'cmd' package. We add it to our test root.
	// Flags for commitCmd (like -m) are defined in its init() function.
	// When commitCmd is added to testRootCmd, its flags should be available.
	testRootCmd.AddCommand(commitCmd)
	// Ensure the -m flag is re-registered if cobra needs it per command instance
    commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
    // No need to MarkFlagRequired again if it's done in init, but safe for isolated test setup.
    // If not done, flag parsing might fail.

	_, err = executeCommandTest(testRootCmd, "commit", "-m", testCommitMsg)
	if err != nil {
		t.Fatalf("commitCmd execution failed: %v", err)
	}

	// 3. Verify .fsegit/index is now empty or does not exist
	_, statErr := os.Stat(indexFilePath)
	if statErr == nil { // File exists
		idxData, _ := ioutil.ReadFile(indexFilePath)
		if len(strings.TrimSpace(string(idxData))) != 0 {
			t.Errorf("Index file was not cleared after commit. Content: %s", string(idxData))
		}
	} else if !os.IsNotExist(statErr) { // Some other error
		t.Errorf("Error checking index file after commit: %v", statErr)
	}

	// 4. Verify .fsegit/HEAD
	headFilePath := filepath.Join(fsegitDir, "HEAD")
	headData, err := ioutil.ReadFile(headFilePath)
	if err != nil {
		t.Fatalf("Failed to read HEAD file: %v", err)
	}
	commitSha1Str := strings.TrimSpace(string(headData))
	if len(commitSha1Str) != 40 {
		t.Fatalf("HEAD content is not a 40-character SHA: got '%s'", commitSha1Str)
	}

	// 5. Verify .fsegit/refs/heads/main
	mainRefPath := filepath.Join(refsHeadsDir, "main")
	mainRefData, err := ioutil.ReadFile(mainRefPath)
	if err != nil {
		t.Fatalf("Failed to read refs/heads/main file: %v", err)
	}
	if strings.TrimSpace(string(mainRefData)) != commitSha1Str {
		t.Errorf("refs/heads/main content mismatch: got '%s', want '%s'", strings.TrimSpace(string(mainRefData)), commitSha1Str)
	}

	// 6. Verify commit object
	commitObjectData, err := readObjectTest(objectsDir, commitSha1Str)
	if err != nil {
		t.Fatalf("Failed to read commit object (SHA: %s): %v", commitSha1Str, err)
	}
	commitParts := strings.SplitN(string(commitObjectData), "\x00", 2)
	if len(commitParts) != 2 {
		t.Fatalf("Invalid commit object format: %s", string(commitObjectData))
	}
	commitBody := commitParts[1]

	if !strings.Contains(commitBody, fmt.Sprintf("\n\n%s", testCommitMsg)) {
		t.Errorf("Commit message not found or incorrect. Body:\n%s", commitBody)
	}
	// Example check for author, adapt as needed for exact format
	if !strings.Contains(commitBody, "author fsegit_user <fsegit@example.com>") {
		t.Errorf("Author info not found or incorrect. Body:\n%s", commitBody)
	}

	treeSha1FromCommit := ""
	lines := strings.Split(commitBody, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			treeSha1FromCommit = strings.Fields(line)[1]
			break
		}
	}
	if treeSha1FromCommit == "" {
		t.Fatalf("Tree SHA not found in commit object. Body:\n%s", commitBody)
	}

	// 7. Verify tree object
	treeObjectData, err := readObjectTest(objectsDir, treeSha1FromCommit)
	if err != nil {
		t.Fatalf("Failed to read tree object (SHA: %s): %v", treeSha1FromCommit, err)
	}
	treeParts := strings.SplitN(string(treeObjectData), "\x00", 2)
	if len(treeParts) != 2 {
		t.Fatalf("Invalid tree object format: %s", string(treeObjectData))
	}
	rawTreeEntries := treeParts[1]

	// Expected tree entry for fileC.txt
	var expectedTreeContent bytes.Buffer
	sha1FileCBytes, _ := hex.DecodeString(blobCSha1) // blobCSha1 was calculated for the blob
	expectedTreeContent.WriteString(fmt.Sprintf("100644 %s\x00", filepath.Base(filePathC)))
	expectedTreeContent.Write(sha1FileCBytes)

	if rawTreeEntries != expectedTreeContent.String() {
		t.Errorf("Tree object content mismatch.\nGot (hex for bytes):\n%x\nWant (hex for bytes):\n%x", []byte(rawTreeEntries), expectedTreeContent.Bytes())
	}

	t.Log("Commit command standalone test verification complete.")
}
