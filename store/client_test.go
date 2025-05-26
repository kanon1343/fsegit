package store

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// コミットオブジェクトが正しく取れるか
func TestClient_GetObject(t *testing.T) {
	// Use a relative path for testing
	testRepoPath := "./testdata/Atcoder"
	// Create the directory if it doesn't exist (basic setup for the test)
	// For a real test suite, more robust setup/teardown might be needed
	// e.g., os.MkdirAll(testRepoPath, 0755)
	// and potentially os.RemoveAll(testRepoPath) in a cleanup function
	client, err := NewClient(testRepoPath)
	if err != nil {
		t.Fatal(err)
	}
	hashString := "ecc8dcee9fc216c5602d369d62bef7d8fdce41d9"
	hash, err := hex.DecodeString(hashString)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := client.GetObject(hash)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprint(obj.Type))
}
