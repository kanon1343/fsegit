package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/kanon1343/fsegit/sha" // For sha.SHA1 type
	"github.com/kanon1343/fsegit/util"

	"github.com/spf13/cobra"
)

var commitMessage string

// commitCmd represents the commit command
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Record changes to the repository",
	Long: `Creates a new commit containing the current contents of the index
and the given log message describing the changes.`,
	Run: func(cmd *cobra.Command, args []string) {
		if commitMessage == "" {
			// TODO: Implement opening an editor for commit message if -m is not provided.
			log.Fatal("Aborting commit due to empty commit message.")
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

		if len(idx.Entries) == 0 {
			log.Println("Nothing to commit (index is empty).")
			return
		}

		// 1. Build the tree object from the index
		rootDirEntries := make(map[string]*store.IndexEntry)
		// Store all entries that are in any subdirectory, keyed by their full path.
		// buildTreeObjectsRecursive will then filter these for the current recursion level.
		allSubDirIndexEntries := make(map[string]*store.IndexEntry) 

		for _, entry := range idx.Entries {
			if strings.Contains(entry.PathName, "/") {
				allSubDirIndexEntries[entry.PathName] = entry
			} else {
				rootDirEntries[entry.PathName] = entry
			}
		}
		
		rootTreeHash, err := buildTreeObjectsRecursive(rootDirEntries, allSubDirIndexEntries, storeClient, "")
		if err != nil {
			log.Fatalf("Failed to build tree objects: %v", err)
		}
		// fmt.Printf("Root tree hash: %s
", rootTreeHash.String())

		// 2. Determine parent commit
		headPath := filepath.Join(dotGitPath, "HEAD")
		headContent, err := ioutil.ReadFile(headPath)
		if err != nil {
			log.Fatalf("Failed to read HEAD: %v", err)
		}

		var parentCommitSHA sha.SHA1
		currentBranchRefPath := ""

		headRefPrefix := "ref: "
		if strings.HasPrefix(string(headContent), headRefPrefix) {
			refName := strings.TrimSpace(string(headContent)[len(headRefPrefix):])
			currentBranchRefPath = filepath.Join(dotGitPath, refName)
			parentCommitSHABytes, err := ioutil.ReadFile(currentBranchRefPath)
			if err != nil {
				if os.IsNotExist(err) {
					// This is the first commit on this branch (or repo)
					parentCommitSHA = nil 
				} else {
					log.Fatalf("Failed to read ref %s: %v", refName, err)
				}
			} else {
				parentCommitSHA, err = sha.FromString(strings.TrimSpace(string(parentCommitSHABytes)))
				if err != nil {
					log.Fatalf("Invalid parent commit SHA in %s: %v", refName, err)
				}
			}
		} else { // Detached HEAD state - HEAD directly contains the commit SHA
			parentCommitSHA, err = sha.FromString(strings.TrimSpace(string(headContent)))
			if err != nil {
				log.Fatalf("Invalid commit SHA in detached HEAD: %v", err)
			}
		}

		// 3. Author and Committer information (placeholders)
		// TODO: Get this from config or environment variables
		authorName := "Test User"
		authorEmail := "test@example.com"
		authorTime := time.Now()
		
		// 4. Construct commit data
		var commitData bytes.Buffer
		fmt.Fprintf(&commitData, "tree %s\n", rootTreeHash.String())
		if parentCommitSHA != nil {
			fmt.Fprintf(&commitData, "parent %s\n", parentCommitSHA.String())
		}
		// Format: "Name <email> timestamp offset"
		// Offset format: +HHMM or -HHMM
		_, offsetSeconds := authorTime.Zone()
		offsetHours := offsetSeconds / 3600
		offsetMinutes := (offsetSeconds % 3600) / 60
		offsetStr := fmt.Sprintf("%+03d%02d", offsetHours, offsetMinutes)

		fmt.Fprintf(&commitData, "author %s <%s> %d %s\n", authorName, authorEmail, authorTime.Unix(), offsetStr)
		fmt.Fprintf(&commitData, "committer %s <%s> %d %s\n", authorName, authorEmail, authorTime.Unix(), offsetStr) // Same for now
		fmt.Fprintf(&commitData, "\n%s\n", commitMessage)

		// 5. Create and write commit object
		commitObj := &object.Object{ // Using direct object.Object as NewBlob does
			Type: object.CommitObject, // Assuming CommitObject is defined in object package
			Size: commitData.Len(),
			Data: commitData.Bytes(),
		}
		// Calculate hash for the commit object
		commitHeaderBytes := commitObj.Header()
		contentToHash := append(commitHeaderBytes, commitObj.Data...)
		hashVal := sha1.Sum(contentToHash)
		commitObj.Hash = hashVal[:]

		if err := storeClient.WriteObject(commitObj); err != nil {
			log.Fatalf("Failed to write commit object: %v", err)
		}
		// fmt.Printf("Commit object created: %s
", commitObj.Hash.String())

		// 6. Update ref (e.g., .git/refs/heads/master or HEAD if detached)
		var refToUpdate string
		if currentBranchRefPath != "" { // Update branch ref
			refToUpdate = currentBranchRefPath
		} else { // Update detached HEAD
			refToUpdate = headPath
		}

		if err := os.MkdirAll(filepath.Dir(refToUpdate), 0755); err != nil {
			log.Fatalf("Failed to create directory for ref %s: %v", refToUpdate, err)
		}
		if err := ioutil.WriteFile(refToUpdate, []byte(commitObj.Hash.String()+"\n"), 0644); err != nil {
			log.Fatalf("Failed to update ref %s: %v", refToUpdate, err)
		}
		
		branchName := ""
		if currentBranchRefPath != "" {
			branchName = filepath.Base(currentBranchRefPath) // Simple name for branch
			if strings.HasPrefix(refToUpdate, dotGitPath)) { // Make it relative for display
				displayPath, _ := filepath.Rel(dotGitPath, refToUpdate)
				branchName = displayPath
			}
		} else {
			branchName = "detached HEAD"
		}


		fmt.Printf("[%s (commit) %s] %s\n", branchName, commitObj.Hash.String()[:7], strings.Split(commitMessage, "\n")[0])
	},
}

// buildTreeObjectsRecursive creates tree objects for a given level of directory structure.
// entriesInCurrentDir: map of path -> IndexEntry for files/links directly in this level.
// allSubDirIndexEntries: map of full_path_from_repo_root -> IndexEntry for all entries in any subdirectory.
// currentPathPrefix: the path prefix for the current directory being processed (e.g., "src/app"). Empty for root.
func buildTreeObjectsRecursive(
	directEntriesInCurrentDir map[string]*store.IndexEntry,
	allSubDirIndexEntries map[string]*store.IndexEntry,
	storeClient *store.Client,
	currentPathPrefix string,
) (sha.SHA1, error) {
	
	var treeEntries []*object.TreeEntry // Using direct object.TreeEntry

	// Add direct entries (files/symlinks) from current directory
	var sortedDirectEntryNames []string
	for name := range directEntriesInCurrentDir {
		sortedDirectEntryNames = append(sortedDirectEntryNames, name)
	}
	sort.Strings(sortedDirectEntryNames)

	for _, name := range sortedDirectEntryNames {
		indexEntry := directEntriesInCurrentDir[name]
		treeEntry := &object.TreeEntry{
			Mode: fmt.Sprintf("%06o", indexEntry.Mode), 
			Name: filepath.Base(indexEntry.PathName), 
			Hash: indexEntry.Hash,
		}
		treeEntries = append(treeEntries, treeEntry)
	}

	// Identify immediate subdirectories of currentPathPrefix
	immediateSubDirs := make(map[string]bool)
	for fullPath := range allSubDirIndexEntries {
		if strings.HasPrefix(fullPath, currentPathPrefix) && len(currentPathPrefix) < len(fullPath) {
			// Path relative to current directory
			pathRelativeToCurrent := strings.TrimPrefix(fullPath, currentPathPrefix)
			pathRelativeToCurrent = strings.TrimPrefix(pathRelativeToCurrent, "/") // Ensure no leading slash

			firstComponent := strings.SplitN(pathRelativeToCurrent, "/", 2)[0]
			if firstComponent != "" && !strings.Contains(firstComponent, "/") {
				immediateSubDirs[firstComponent] = true
			}
		}
	}
    
    var sortedSubDirNames []string
    for sdName := range immediateSubDirs {
        sortedSubDirNames = append(sortedSubDirNames, sdName)
    }
    sort.Strings(sortedSubDirNames)

	for _, subDirName := range sortedSubDirNames {
		// Prepare entries for the subdirectory to pass to recursive call
		subDirDirectEntriesForNextLevel := make(map[string]*store.IndexEntry)
		// The full path for this immediate subdirectory, used as the new currentPathPrefix
		nextPathPrefix := filepath.Join(currentPathPrefix, subDirName)
		
		// Filter allSubDirIndexEntries for entries that are DIRECTLY in nextPathPrefix
		// or in subdirectories OF nextPathPrefix
		subDirEntriesForNextLevelRecursion := make(map[string]*store.IndexEntry)

		for fullPath, entry := range allSubDirIndexEntries {
			if strings.HasPrefix(fullPath, nextPathPrefix + "/") { // It's in a sub-directory of nextPathPrefix
				subDirEntriesForNextLevelRecursion[fullPath] = entry
			} else if filepath.Dir(fullPath) == nextPathPrefix { // It's directly in nextPathPrefix
				// This logic needs to be careful: filepath.Dir("a/b/c.txt") is "a/b"
				// filepath.Base(entry.PathName) is what we need for map key if map is for current dir only
				subDirDirectEntriesForNextLevel[entry.PathName] = entry
			}
		}
		
		subTreeHash, err := buildTreeObjectsRecursive(subDirDirectEntriesForNextLevel, subDirEntriesForNextLevelRecursion, storeClient, nextPathPrefix)
		if err != nil {
			return nil, fmt.Errorf("failed to build tree for subdir %s: %w", nextPathPrefix, err)
		}
		treeEntry := &object.TreeEntry{
			Mode: "040000", // Directory mode
			Name: subDirName,
			Hash: subTreeHash,
		}
		treeEntries = append(treeEntries, treeEntry)
	}
    
    // If treeEntries is empty (e.g. an empty directory added to index, though git usually doesn't track them)
    // or a directory becomes empty after processing children.
    // Git creates a specific empty tree object in such cases.
    // Our object.NewTree should handle nil/empty entries correctly and produce the standard empty tree.
	currentTreeObj, err := object.NewTree(treeEntries) // Using direct object.NewTree
	if err != nil {
		return nil, fmt.Errorf("failed to create new tree object for path %s: %w", currentPathPrefix, err)
	}

	// For NewTree to calculate hash, it needs to serialize.
	// The object.Object returned by NewTree should have its hash computed.
	if err := storeClient.WriteObject(currentTreeObj); err != nil {
		return nil, fmt.Errorf("failed to write tree object for path %s: %w", currentPathPrefix, err)
	}
	return currentTreeObj.Hash, nil
}


func init() {
	rootCmd.AddCommand(commitCmd)
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message")
}

```
