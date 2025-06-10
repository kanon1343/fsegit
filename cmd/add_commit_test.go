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

func TestAddCommitWorkflow(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "fsegit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}
	defer os.Chdir(originalWd)

	// 1. Initialize fsegit repository structure
	// Use CreateTestRepo helper
	_, objectsDir, refsHeadsDir := CreateTestRepo(t, tempDir)
	fsegitDir := filepath.Join(tempDir, ".fsegit") // CreateTestRepo uses tempDir as root for .fsegit

	// 2. Create sample files
	file1Content := []byte("hello")
	file2Content := []byte("world")
	if err := ioutil.WriteFile("file1.txt", file1Content, 0644); err != nil {
		t.Fatalf("Failed to write file1.txt: %v", err)
	}
	if err := ioutil.WriteFile("file2.txt", file2Content, 0644); err != nil {
		t.Fatalf("Failed to write file2.txt: %v", err)
	}

	// Reset global states for commands if necessary (e.g. flags)
	// This is important because cobra commands can have global state.
	// Create a new root command for this test execution
	testRootCmd := &cobra.Command{Use: "fsegit-test-ac"}
	ResetCommandStatesTest(t, testRootCmd, addCmd, commitCmd)


	// 3. Programmatically execute the addCmd
	_, err = ExecuteCommandTest(testRootCmd, "add", "file1.txt", "file2.txt")
	if err != nil {
		t.Fatalf("addCmd execution failed: %v", err)
	}

	// 4. Verify .fsegit/index
	indexFilePath := filepath.Join(fsegitDir, "index")
	indexData, err := ioutil.ReadFile(indexFilePath)
	if err != nil {
		t.Fatalf("Failed to read index file: %v", err)
	}

	indexEntries := strings.Split(strings.TrimSpace(string(indexData)), "\n")
	if len(indexEntries) != 2 {
		t.Fatalf("Expected 2 entries in index, got %d: %v", len(indexEntries), indexEntries)
	}

	expectedSha1File1 := CalculateBlobSHATest(file1Content)
	expectedSha2File2 := CalculateBlobSHATest(file2Content)
	foundFile1 := false
	foundFile2 := false

	for _, entry := range indexEntries {
		parts := strings.Fields(entry)
		if len(parts) != 2 {
			t.Errorf("Invalid index entry format: '%s'", entry)
			continue
		}
		filePath := parts[0]
		sha1Hash := parts[1]
		if filePath == "file1.txt" {
			if sha1Hash != expectedSha1File1 {
				t.Errorf("file1.txt SHA mismatch: got %s, want %s", sha1Hash, expectedSha1File1)
			}
			foundFile1 = true
		} else if filePath == "file2.txt" {
			if sha1Hash != expectedSha2File2 {
				t.Errorf("file2.txt SHA mismatch: got %s, want %s", sha1Hash, expectedSha2File2)
			}
			foundFile2 = true
		}
	}
	if !foundFile1 {
		t.Errorf("file1.txt not found in index")
	}
	if !foundFile2 {
		t.Errorf("file2.txt not found in index")
	}

	// 5. Verify blob objects
	blob1Data, err := ReadObjectTest(objectsDir, expectedSha1File1)
	if err != nil {
		t.Fatalf("Failed to read blob object for file1.txt (SHA: %s): %v", expectedSha1File1, err)
	}
	expectedBlob1ObjectContent := fmt.Sprintf("blob %d\x00%s", len(file1Content), file1Content)
	if string(blob1Data) != expectedBlob1ObjectContent {
		t.Errorf("file1.txt blob content mismatch: got '%s', want '%s'", string(blob1Data), expectedBlob1ObjectContent)
	}

	blob2Data, err := ReadObjectTest(objectsDir, expectedSha2File2)
	if err != nil {
		t.Fatalf("Failed to read blob object for file2.txt (SHA: %s): %v", expectedSha2File2, err)
	}
	expectedBlob2ObjectContent := fmt.Sprintf("blob %d\x00%s", len(file2Content), file2Content)
	if string(blob2Data) != expectedBlob2ObjectContent {
		t.Errorf("file2.txt blob content mismatch: got '%s', want '%s'", string(blob2Data), expectedBlob2ObjectContent)
	}

	t.Log("Add command verification complete.")

	// 6. Programmatically execute the commitCmd
	// Reset states again for commit, ensuring commitMessage var is clean and flags are set on testRootCmd
	ResetCommandStatesTest(t, testRootCmd, addCmd, commitCmd) // Re-pass addCmd as well or just commitCmd if preferred
	commitTestMessage := "Test commit"
	_, err = ExecuteCommandTest(testRootCmd, "commit", "-m", commitTestMessage)
	if err != nil {
		t.Fatalf("commitCmd execution failed: %v", err)
	}

	// 7. Verify .fsegit/index is now empty or does not exist
	_, err = os.Stat(indexFilePath)
	if err == nil {
		indexData, _ := ioutil.ReadFile(indexFilePath)
		if len(strings.TrimSpace(string(indexData))) != 0 {
			t.Errorf("Index file was not cleared after commit. Content: %s", string(indexData))
		}
	} else if !os.IsNotExist(err) {
		t.Errorf("Error checking index file after commit: %v", err)
	}

	// 8. Verify .fsegit/HEAD
	headFilePath := filepath.Join(fsegitDir, "HEAD")
	headData, err := ioutil.ReadFile(headFilePath)
	if err != nil {
		t.Fatalf("Failed to read HEAD file: %v", err)
	}
	commitSha1Str := strings.TrimSpace(string(headData))
	if len(commitSha1Str) != 40 {
		t.Fatalf("HEAD content is not a 40-character SHA: got '%s'", commitSha1Str)
	}

	// 9. Verify .fsegit/refs/heads/main
	mainRefPath := filepath.Join(refsHeadsDir, "main")
	mainRefData, err := ioutil.ReadFile(mainRefPath)
	if err != nil {
		t.Fatalf("Failed to read refs/heads/main file: %v", err)
	}
	if strings.TrimSpace(string(mainRefData)) != commitSha1Str {
		t.Errorf("refs/heads/main content mismatch: got '%s', want '%s'", strings.TrimSpace(string(mainRefData)), commitSha1Str)
	}

	// 10. Verify commit object
	commitObjectData, err := ReadObjectTest(objectsDir, commitSha1Str)
	if err != nil {
		t.Fatalf("Failed to read commit object (SHA: %s): %v", commitSha1Str, err)
	}

	commitParts := strings.SplitN(string(commitObjectData), "\x00", 2)
	if len(commitParts) != 2 {
		t.Fatalf("Invalid commit object format (no null separator): %s", string(commitObjectData))
	}
	// commitHeader := commitParts[0] // e.g. "commit <size>"
	commitBody := commitParts[1]

	if !strings.Contains(commitBody, fmt.Sprintf("\n\n%s", commitTestMessage)) { // two newlines before message
		t.Errorf("Commit message not found or incorrect in commit object. Body:\n%s", commitBody)
	}
	if !strings.Contains(commitBody, "author fsegit_user <fsegit@example.com>") {
		t.Errorf("Author info not found or incorrect. Body:\n%s", commitBody)
	}

	treeSha1FromCommit := ""
	lines := strings.Split(commitBody, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			treeSha1FromCommit = strings.TrimSpace(strings.TrimPrefix(line, "tree "))
			break
		}
	}
	if treeSha1FromCommit == "" {
		t.Fatalf("Tree SHA not found in commit object. Body:\n%s", commitBody)
	}
	if len(treeSha1FromCommit) != 40 {
		t.Fatalf("Tree SHA in commit object is not a 40-character SHA: got '%s'", treeSha1FromCommit)
	}

	// 11. Verify tree object
	treeObjectData, err := ReadObjectTest(objectsDir, treeSha1FromCommit)
	if err != nil {
		t.Fatalf("Failed to read tree object (SHA: %s): %v", treeSha1FromCommit, err)
	}

	treeParts := strings.SplitN(string(treeObjectData), "\x00", 2)
	if len(treeParts) != 2 {
		t.Fatalf("Invalid tree object format (no null separator for header): %s", string(treeObjectData))
	}
	// treeHeader := treeParts[0] // e.g. "tree <size>"
	rawTreeEntries := treeParts[1]

	// Expected entries (sorted by name)
	// file1.txt SHA: expectedSha1File1
	// file2.txt SHA: expectedSha2File2

	// Manually construct the expected raw tree content for comparison
	// Entry format: <mode> <name><sha1_bytes>
	var expectedTreeContent bytes.Buffer

	sha1File1Bytes := DecodeSHA1Hex(t, expectedSha1File1)
	expectedTreeContent.WriteString(fmt.Sprintf("100644 file1.txt\x00"))
	expectedTreeContent.Write(sha1File1Bytes)

	sha1File2Bytes := DecodeSHA1Hex(t, expectedSha2File2)
	expectedTreeContent.WriteString(fmt.Sprintf("100644 file2.txt\x00"))
	expectedTreeContent.Write(sha1File2Bytes)

	if rawTreeEntries != expectedTreeContent.String() {
		t.Errorf("Tree object content mismatch.\nGot (hex for bytes):\n%x\nWant (hex for bytes):\n%x", []byte(rawTreeEntries), expectedTreeContent.Bytes())
		t.Logf("Got string: %s", rawTreeEntries)
		t.Logf("Want string: %s", expectedTreeContent.String())

	}

	t.Log("Commit command and overall workflow verification complete.")
}
