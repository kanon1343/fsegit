package cmd

import (
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [files...]",
	Short: "Add file contents to the index",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, filePath := range args {
			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", filePath, err)
			}

			// Create blob object
			header := fmt.Sprintf("blob %d\x00", len(content))
			blobData := append([]byte(header), content...)

			// Calculate SHA1 hash
			hash := sha1.Sum(blobData)
			sha1Str := fmt.Sprintf("%x", hash)

			// Store blob object
			objectDir := filepath.Join(".fsegit", "objects", sha1Str[:2])
			objectPath := filepath.Join(objectDir, sha1Str[2:])

			if _, err := os.Stat(objectDir); os.IsNotExist(err) {
				if err := os.MkdirAll(objectDir, 0755); err != nil {
					return fmt.Errorf("failed to create object directory %s: %w", objectDir, err)
				}
			}

			// Compress and write blob
			objectFile, err := os.Create(objectPath)
			if err != nil {
				return fmt.Errorf("failed to create object file %s: %w", objectPath, err)
			}
			defer objectFile.Close()

			zlibWriter := zlib.NewWriter(objectFile)
			if _, err := zlibWriter.Write(blobData); err != nil {
				return fmt.Errorf("failed to write compressed data to object file %s: %w", objectPath, err)
			}
			if err := zlibWriter.Close(); err != nil {
				return fmt.Errorf("failed to close zlib writer for object file %s: %w", objectPath, err)
			}

			// Update index
			indexFilePath := filepath.Join(".fsegit", "index")
			indexData, err := ioutil.ReadFile(indexFilePath)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to read index file %s: %w", indexFilePath, err)
			}

			lines := strings.Split(string(indexData), "\n")
			newLines := make([]string, 0, len(lines))
			found := false
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) == 2 && parts[0] == filePath {
					newLines = append(newLines, fmt.Sprintf("%s %s", filePath, sha1Str))
					found = true
				} else {
					newLines = append(newLines, line)
				}
			}

			if !found {
				newLines = append(newLines, fmt.Sprintf("%s %s", filePath, sha1Str))
			}

			if err := ioutil.WriteFile(indexFilePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
				return fmt.Errorf("failed to write updated index file %s: %w", indexFilePath, err)
			}

			fmt.Printf("Added %s to index with SHA %s\n", filePath, sha1Str)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
