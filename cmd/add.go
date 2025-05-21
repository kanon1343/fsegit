package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings" // For strings.HasPrefix and filepath.ToSlash
	"syscall"
	"time"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/kanon1343/fsegit/util"

	"github.com/spf13/cobra"
)

const (
	FileModeRegular    = 0100644
	FileModeExecutable = 0100755
)

var addCmd = &cobra.Command{
	Use:   "add <pathspec>...",
	Short: "Add file contents to the index",
	Long: `This command updates the index using the current content found in the working tree,
to prepare the content staged for the next commit.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			log.Println("Nothing specified, nothing added.")
			return
		}

		gitDir, err := util.FindGitRoot(".")
		if err != nil {
			log.Fatalf("fatal: not a git repository (or any of the parent directories): .git")
		}
		dotGitPath := filepath.Join(gitDir, ".git")

		storeClient, err := store.NewClient(gitDir)
		if err != nil {
			log.Fatalf("Failed to create store client: %v", err)
		}

		idx, err := store.ReadIndex(dotGitPath)
		if err != nil {
			log.Fatalf("Failed to read index: %v", err)
		}

		for _, pathSpec := range args {
			absPath, err := filepath.Abs(pathSpec)
			if err != nil {
				log.Printf("Error getting absolute path for %s: %v. Skipping.", pathSpec, err)
				continue
			}

			relPath, err := filepath.Rel(gitDir, absPath)
			if err != nil {
				log.Printf("Error getting relative path for %s: %v. Skipping.", pathSpec, err)
				continue
			}
			// Ensure consistent slash representation for cross-platform compatibility in index
			relPath = filepath.ToSlash(relPath)
			
			if strings.HasPrefix(relPath, "..") {
				log.Printf("Path %s is outside the current repository. Skipping.", pathSpec)
				continue
			}

			fileInfo, err := os.Lstat(absPath) // Use Lstat to not follow symlinks initially
			if err != nil {
				log.Printf("Failed to stat %s: %v. Skipping.", pathSpec, err)
				continue
			}

			if fileInfo.IsDir() {
				log.Printf("Path %s is a directory. Recursive add not yet implemented. Skipping.", pathSpec)
				continue
			}
			if fileInfo.Mode()&os.ModeSymlink != 0 {
				log.Printf("Path %s is a symlink. Symlink support not yet implemented. Skipping.", pathSpec)
				continue
			}

			fileData, err := ioutil.ReadFile(absPath)
			if err != nil {
				log.Printf("Failed to read file %s: %v. Skipping.", pathSpec, err)
				continue
			}

			blobObj := object.NewBlob(fileData)
			if err := storeClient.WriteObject(blobObj); err != nil {
				log.Printf("Failed to write blob object for %s: %v. Skipping.", relPath, err)
				continue
			}

			entry := &store.IndexEntry{}

			// Get syscall.Stat_t for ctime, dev, ino, uid, gid
			// Note: fileInfo.Sys() can be nil if not supported or if os.Lstat failed in a weird way
			sysStat, ok := fileInfo.Sys().(*syscall.Stat_t)
			if !ok {
				log.Printf("Failed to get syscall.Stat_t for %s (fileInfo.Sys() is nil or not *syscall.Stat_t). Skipping.", relPath)
				continue
			}

			entry.CTimeSeconds = uint32(sysStat.Ctim.Sec)
			entry.CTimeNanoseconds = uint32(sysStat.Ctim.Nsec)
			entry.MTimeSeconds = uint32(fileInfo.ModTime().Unix())
			entry.MTimeNanoseconds = uint32(fileInfo.ModTime().Nanosecond())
			entry.Dev = uint32(sysStat.Dev)
			entry.Ino = uint32(sysStat.Ino)

			if fileInfo.Mode()&0111 != 0 { // Check for execute permission for any of user, group, or other
				entry.Mode = FileModeExecutable
			} else {
				entry.Mode = FileModeRegular
			}

			entry.UID = sysStat.Uid
			entry.GID = sysStat.Gid
			entry.Size = uint32(fileInfo.Size())
			entry.Hash = blobObj.Hash
			entry.PathName = relPath // Already converted to slash path

			// As per instructions:
			// The `flags` field of IndexEntry is private in store/index.go.
			// We are relying on store.WriteIndex to correctly calculate 
			// the flags (path length and assuming stage 0) from entry.PathName when writing the index.
			// No direct setting of entry.Flags here.
			// If store.IndexEntry.Flags was made public and a Setter method like SetPackedFlags exists,
			// it would be called here: entry.SetPackedFlags(0, len(entry.PathName))

			idx.AddEntry(entry)
			fmt.Printf("Added '%s' to index.\n", entry.PathName)
		}

		if err := store.WriteIndex(idx); err != nil {
			log.Fatalf("Failed to write updated index: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
