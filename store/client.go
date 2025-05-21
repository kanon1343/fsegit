package store

import (
	"bytes" // Required for zlib.NewWriter if it expects an io.Writer
	"compress/zlib"
	"fmt" // Required for obj.Header() if not aliasing object package
	"io/ioutil"
	"os"
	"path/filepath"

	objectspec "github.com/kanon1343/fsegit/object" // Alias for object package
	"github.com/kanon1343/fsegit/sha"
	"github.com/kanon1343/fsegit/util"
)

type Client struct {
	objectDir string
}

// NewClient finds the repository's root directory and sets up the client.
func NewClient(path string) (*Client, error) {
	rootDir, err := util.FindGitRoot(path)
	if err != nil {
		return nil, err
	}
	return &Client{
		objectDir: filepath.Join(rootDir, ".git", "objects"),
	}, nil
}

// GetObject retrieves an object by its hash from the object store.
func (c *Client) GetObject(hash sha.SHA1) (*objectspec.Object, error) {
	hashString := hash.String()
	if len(hashString) != 40 { // sha.SHA1.String() should produce a 40-char hex string
		return nil, fmt.Errorf("invalid hash string format: %s", hashString)
	}
	objectPath := filepath.Join(c.objectDir, hashString[:2], hashString[2:])

	objectFile, err := os.Open(objectPath)
	if err != nil {
		// Distinguish between file not found and other errors
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object %s not found: %w", hashString, err)
		}
		return nil, fmt.Errorf("failed to open object file %s: %w", objectPath, err)
	}
	defer objectFile.Close()

	zr, err := zlib.NewReader(objectFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader for %s: %w", objectPath, err)
	}
	defer zr.Close() // Important to close the zlib.Reader

	// Use objectspec.ReadObject from the aliased package
	obj, err := objectspec.ReadObject(zr)
	if err != nil {
		return nil, fmt.Errorf("failed to read object %s: %w", hashString, err)
	}
	// Verify that the read object's hash matches the requested hash
	// This is implicitly done by ReadObject as it calculates hash from content.
	// If we want to be explicit:
	// if !bytes.Equal(obj.Hash, hash) {
	//    return nil, fmt.Errorf("hash mismatch for object %s: expected %s, got %s", hashString, hash.String(), obj.Hash.String())
	// }
	return obj, nil
}

// WriteObject saves an object (blob, tree, commit) to the object store.
// The object's hash should already be computed and stored in obj.Hash.
func (c *Client) WriteObject(obj *objectspec.Object) error {
	hashStr := obj.Hash.String()
	if len(hashStr) != 40 {
		return fmt.Errorf("invalid hash string for WriteObject: %s", hashStr)
	}

	dirPath := filepath.Join(c.objectDir, hashStr[:2])
	filePath := filepath.Join(dirPath, hashStr[2:])

	// Check if object already exists to avoid re-writing
	if _, err := os.Stat(filePath); err == nil {
		// Object already exists, no need to write again.
		// Depending on strictness, could verify content if concerned about collisions,
		// but typically Git assumes SHA1 uniqueness.
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error with stat (e.g., permission issue)
		return fmt.Errorf("failed to stat object file %s: %w", filePath, err)
	}

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create object directory %s: %w", dirPath, err)
	}

	// Create file in a temporary name first, then rename to avoid partial writes
	tempFile, err := ioutil.TempFile(dirPath, "tmp_obj_")
	if err != nil {
		return fmt.Errorf("failed to create temporary object file in %s: %w", dirPath, err)
	}
	// tempFile.Name() has the full path to the temporary file

	closed := false // Flag to ensure tempFile is closed once
	defer func() {
		if !closed {
			tempFile.Close() // Close if not already closed
		}
		if err != nil { // If an error occurred during write/rename, remove temp file
			os.Remove(tempFile.Name())
		}
	}()

	zw := zlib.NewWriter(tempFile)

	// Write header
	headerBytes := obj.Header() // This comes from objectspec.Object's method
	if _, err := zw.Write(headerBytes); err != nil {
		// Error already includes tempFile.Name() context via defer logic for os.Remove
		return fmt.Errorf("failed to write object header: %w", err)
	}

	// Write data
	if _, err := zw.Write(obj.Data); err != nil {
		return fmt.Errorf("failed to write object data: %w", err)
	}

	if err := zw.Close(); err != nil { // Must close zlib.Writer to flush all data
		return fmt.Errorf("failed to close zlib writer: %w", err)
	}

	if err := tempFile.Close(); err != nil { // Close the file itself
		closed = true
		return fmt.Errorf("failed to close temporary object file: %w", err)
	}
	closed = true

	// Rename temporary file to final path
	if err := os.Rename(tempFile.Name(), filePath); err != nil {
		// If rename fails, the temp file still exists and will be removed by the defer
		return fmt.Errorf("failed to rename temporary object file from %s to %s: %w", tempFile.Name(), filePath, err)
	}

	return nil
}

// WalkHistory is assumed to be already implemented correctly.
// (It uses GetObject and objectspec.NewCommit)
func (c *Client) WalkHistory(hash sha.SHA1, walkFunc objectspec.WalkFunc) error {
	ancestors := []sha.SHA1{hash}
	// Keep track of visited commits to avoid cycles and redundant processing
	visited := make(map[string]struct{})

	for len(ancestors) > 0 {
		currentHash := ancestors[0]
		ancestors = ancestors[1:] // Dequeue

		hashStr := currentHash.String()
		if _, ok := visited[hashStr]; ok {
			continue // Already visited
		}
		visited[hashStr] = struct{}{}

		obj, err := c.GetObject(currentHash)
		if err != nil {
			// If an object is not found, it might be an error or end of a line of history
			// Depending on desired strictness, this could be a fatal error or skipped
			return fmt.Errorf("failed to get object %s during history walk: %w", hashStr, err)
		}

		// Use objectspec.NewCommit from the aliased package
		commit, err := objectspec.NewCommit(obj)
		if err != nil {
			return fmt.Errorf("failed to parse commit %s: %w", hashStr, err)
		}

		if err := walkFunc(commit); err != nil {
			// Allow walkFunc to stop the walk by returning an error
			if err == objectspec.ErrStopWalk { // Define ErrStopWalk in object package if needed
				return nil
			}
			return fmt.Errorf("error in walk function for commit %s: %w", hashStr, err)
		}

		// Enqueue parents if they haven't been visited
		for _, parentHash := range commit.Parents {
			if _, ok := visited[parentHash.String()]; !ok {
				ancestors = append(ancestors, parentHash)
			}
		}
	}
	return nil
}
