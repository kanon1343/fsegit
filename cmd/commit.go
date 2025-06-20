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
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var commitMessage string

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Record changes to the repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if commitMessage == "" {
			return fmt.Errorf("commit message is required (-m)")
		}

		// Ensure .fsegit directory and .fsegit/objects exist
		if err := os.MkdirAll(filepath.Join(".fsegit", "objects"), 0755); err != nil {
			return fmt.Errorf("failed to create .fsegit/objects directory: %w", err)
		}

		// Read staged files from .fsegit/index
		indexFilePath := filepath.Join(".fsegit", "index")
		indexData, err := ioutil.ReadFile(indexFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("index is empty, nothing to commit")
			}
			return fmt.Errorf("failed to read index file %s: %w", indexFilePath, err)
		}

		trimmedIndexData := strings.TrimSpace(string(indexData))
		if trimmedIndexData == "" {
			return fmt.Errorf("index is empty, nothing to commit")
		}
		indexEntries := strings.Split(trimmedIndexData, "\n")

		// Create tree object
		// For now, support a flat directory structure
		
		// Define a struct to hold tree entry data for proper sorting
		type treeEntryData struct {
			mode     string
			name     string
			sha1Bytes []byte
		}
		var parsedTreeEntries []treeEntryData

		for _, entry := range indexEntries {
			parts := strings.Fields(entry)
			if len(parts) != 2 {
				return fmt.Errorf("invalid index entry: %s", entry)
			}
			filePath := parts[0]
			sha1Hex := parts[1]
			
			sha1Bytes, err := hex.DecodeString(sha1Hex)
			if err != nil {
				return fmt.Errorf("failed to decode sha1 hex %s for file %s: %w", sha1Hex, filePath, err)
			}

			fileName := filepath.Base(filePath)
			parsedTreeEntries = append(parsedTreeEntries, treeEntryData{
				mode:     "100644",
				name:     fileName,
				sha1Bytes: sha1Bytes,
			})
		}

		// Sort entries by name
		sort.Slice(parsedTreeEntries, func(i, j int) bool {
			return parsedTreeEntries[i].name < parsedTreeEntries[j].name
		})

		var treeContentBuffer bytes.Buffer
		for _, te := range parsedTreeEntries {
			treeContentBuffer.WriteString(fmt.Sprintf("%s %s\x00", te.mode, te.name))
			treeContentBuffer.Write(te.sha1Bytes)
		}
		
		treeContentBytes := treeContentBuffer.Bytes()
		// Tree object format: tree <content_size><entries>
		treeHeader := fmt.Sprintf("tree %d\x00", len(treeContentBytes))
		treeObjectData := append([]byte(treeHeader), treeContentBytes...)

		// Calculate SHA1 of the tree object data
		treeSha1 := sha1.Sum(treeObjectData)
		treeSha1Str := fmt.Sprintf("%x", treeSha1)

		// Store the tree object in .fsegit/objects
		if err := storeObject(treeSha1Str, treeObjectData); err != nil {
			return fmt.Errorf("failed to store tree object: %w", err)
		}

		// Get parent commit SHA
		headFilePath := filepath.Join(".fsegit", "HEAD")
		parentSha1Str := ""
		headData, err := ioutil.ReadFile(headFilePath)
		if err == nil && len(strings.TrimSpace(string(headData))) > 0 {
			parentSha1Str = strings.TrimSpace(string(headData))
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read HEAD file: %w", err)
		}
		
		// Construct commit object data
		authorName := "fsegit_user"
		authorEmail := "fsegit@example.com"
		now := time.Now()
		timestamp := now.Unix()
		_, offsetSeconds := now.Zone()
		timezoneOffset := fmt.Sprintf("%+03d%02d", offsetSeconds/3600, (offsetSeconds%3600)/60)

		var commitObjectParts []string
		commitObjectParts = append(commitObjectParts, fmt.Sprintf("tree %s", treeSha1Str))
		if parentSha1Str != "" {
			commitObjectParts = append(commitObjectParts, fmt.Sprintf("parent %s", parentSha1Str))
		}
		commitObjectParts = append(commitObjectParts, fmt.Sprintf("author %s <%s> %d %s", authorName, authorEmail, timestamp, timezoneOffset))
		commitObjectParts = append(commitObjectParts, fmt.Sprintf("committer %s <%s> %d %s", authorName, authorEmail, timestamp, timezoneOffset))
		commitObjectParts = append(commitObjectParts, "") // Empty line before commit message
		commitObjectParts = append(commitObjectParts, commitMessage)

		commitContent := strings.Join(commitObjectParts, "\n")
		commitHeader := fmt.Sprintf("commit %d\x00", len(commitContent))
		commitObjectData := append([]byte(commitHeader), []byte(commitContent)...)
		
		// Calculate SHA1 of the commit object data
		commitSha1 := sha1.Sum(commitObjectData)
		commitSha1Str := fmt.Sprintf("%x", commitSha1)

		// Store the commit object
		if err := storeObject(commitSha1Str, commitObjectData); err != nil {
			return fmt.Errorf("failed to store commit object: %w", err)
		}

		// Update HEAD
		if err := ioutil.WriteFile(headFilePath, []byte(commitSha1Str+"\n"), 0644); err != nil {
			return fmt.Errorf("failed to write to HEAD: %w", err)
		}

		// Update refs/heads/main (simplified)
		mainRefPath := filepath.Join(".fsegit", "refs", "heads", "main")
		if err := os.MkdirAll(filepath.Dir(mainRefPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for main ref: %w", err)
		}
		if err := ioutil.WriteFile(mainRefPath, []byte(commitSha1Str+"\n"), 0644); err != nil {
			return fmt.Errorf("failed to write to main ref: %w", err)
		}
		
		// Clear the index
		if err := os.Remove(indexFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clear index: %w", err)
		}

		fmt.Printf("[%s] %s\n", commitSha1Str[:7], strings.Split(commitMessage, "\n")[0])
		return nil
	},
}

// storeObject compresses and stores an object in the .fsegit/objects directory
func storeObject(sha1Str string, data []byte) error {
	objectDir := filepath.Join(".fsegit", "objects", sha1Str[:2])
	objectPath := filepath.Join(objectDir, sha1Str[2:])

	if _, err := os.Stat(objectDir); os.IsNotExist(err) {
		if err := os.MkdirAll(objectDir, 0755); err != nil {
			return fmt.Errorf("failed to create object directory %s: %w", objectDir, err)
		}
	}

	objectFile, err := os.Create(objectPath)
	if err != nil {
		return fmt.Errorf("failed to create object file %s: %w", objectPath, err)
	}
	defer objectFile.Close()

	zlibWriter := zlib.NewWriter(objectFile)
	if _, err := zlibWriter.Write(data); err != nil {
		return fmt.Errorf("failed to write compressed data to object file %s: %w", objectPath, err)
	}
	if err := zlibWriter.Close(); err != nil {
		return fmt.Errorf("failed to close zlib writer for object file %s: %w", objectPath, err)
	}
	return nil
}


func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	if err:= commitCmd.MarkFlagRequired("message"); err != nil {
		// This should not happen in init, but good practice to check.
		// In a real scenario, this would be handled or logged.
		fmt.Fprintf(os.Stderr, "Error marking 'message' flag required: %v\n", err)
	}
	rootCmd.AddCommand(commitCmd)
}
