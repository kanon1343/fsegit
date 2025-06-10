package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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

	fsegitDir, objectsDir, refsHeadsDir := CreateTestRepo(t, tempDir)

	// 1. Prepare for commit (simulate 'fsegit add')
	fileCContent := []byte("This is file C for commit test.")
	filePathC := "fileC.txt"
	if err := ioutil.WriteFile(filePathC, fileCContent, 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", filePathC, err)
	}

	// Manually create blob object for fileC.txt
	blobCSha1 := CalculateBlobSHATest(fileCContent)
	blobCHeader := fmt.Sprintf("blob %d\x00", len(fileCContent))
	blobCData := append([]byte(blobCHeader), fileCContent...)
	StoreObjectTest(t, objectsDir, blobCSha1, blobCData)

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
	testRootCmd := &cobra.Command{Use: "fsegit-test"}
	ResetCommandStatesTest(t, testRootCmd, commitCmd)


	// commitCmd is a global var in the 'cmd' package. We add it to our test root.
	// Flags for commitCmd (like -m) are defined in its init() function.
	// When commitCmd is added to testRootCmd, its flags should be available.
	// Ensure the -m flag is re-registered if cobra needs it per command instance - This is handled by ResetCommandStatesTest

	_, err = ExecuteCommandTest(testRootCmd, "commit", "-m", testCommitMsg)
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
	commitObjectData, err := ReadObjectTest(objectsDir, commitSha1Str)
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
	treeObjectData, err := ReadObjectTest(objectsDir, treeSha1FromCommit)
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
	sha1FileCBytes := DecodeSHA1Hex(t, blobCSha1) // blobCSha1 was calculated for the blob
	expectedTreeContent.WriteString(fmt.Sprintf("100644 %s\x00", filepath.Base(filePathC)))
	expectedTreeContent.Write(sha1FileCBytes)

	if rawTreeEntries != expectedTreeContent.String() {
		t.Errorf("Tree object content mismatch.\nGot (hex for bytes):\n%x\nWant (hex for bytes):\n%x", []byte(rawTreeEntries), expectedTreeContent.Bytes())
	}

	t.Log("Commit command standalone test verification complete.")
}
