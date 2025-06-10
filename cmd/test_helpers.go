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

// ExecuteCommandTest executes cobra commands for testing.
func ExecuteCommandTest(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return strings.TrimSpace(buf.String()), err
}

// CalculateBlobSHATest calculates SHA1 for blob content.
func CalculateBlobSHATest(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	data := append([]byte(header), content...)
	hash := sha1.Sum(data)
	return fmt.Sprintf("%x", hash)
}

// StoreObjectTest compresses and stores data, mimicking storeObject from commit.go
func StoreObjectTest(t *testing.T, objectsDir string, sha1Str string, data []byte) {
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

// ReadObjectTest reads and decompresses an object file.
func ReadObjectTest(objectDir, sha1Str string) ([]byte, error) {
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

// CreateTestRepo sets up a basic .fsegit structure.
// It returns fsegitDir, objectsDir, and refsHeadsDir
func CreateTestRepo(t *testing.T, tempDir string) (string, string, string) {
	t.Helper()
	fsegitDir := filepath.Join(tempDir, ".fsegit")
	objectsDir := filepath.Join(fsegitDir, "objects")
	refsHeadsDir := filepath.Join(fsegitDir, "refs", "heads")

	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", objectsDir, err)
	}
	if err := os.MkdirAll(refsHeadsDir, 0755); err != nil {
		t.Fatalf("Failed to create %s: %v", refsHeadsDir, err)
	}
	return fsegitDir, objectsDir, refsHeadsDir
}

// ResetCommitCmdState resets state for commitCmd, specifically the message flag.
func ResetCommitCmdState() {
	commitMessage = "" // This is the package-level variable for the -m flag in commit.go
}

// ResetCommandStatesTest resets global state for commands, particularly for flags or package-level vars.
// This is a more generic version, potentially adaptable.
// For now, it primarily resets commitMessage for commitCmd if used in a test suite
// and re-initializes a given root command with its children.
func ResetCommandStatesTest(t *testing.T, testRootCmd *cobra.Command, targetCmds ...*cobra.Command) {
    t.Helper()
    // Reset package-level flags
    commitMessage = "" // Example: reset commit message flag

    // Clear existing commands from the testRootCmd
    testRootCmd.ResetCommands()

    // Add target commands (like addCmd, commitCmd) to the testRootCmd
    for _, cmd := range targetCmds {
        testRootCmd.AddCommand(cmd)
    }

    // Special handling for commitCmd flags if it's one of the target commands
    // We only need to reset the variable, not redefine the flag.
    // The flag definition should remain in the commitCmd's init() function.
}

// Helper function to decode hex string and handle error for tests
func DecodeSHA1Hex(t *testing.T, hexStr string) []byte {
	t.Helper()
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("Failed to decode hex string '%s': %v", hexStr, err)
	}
	return bytes
}
