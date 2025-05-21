package cmd

import (
	"fmt"
	"log"
	"path/filepath" // For joining paths to .git/index

	"github.com/kanon1343/fsegit/store" // For ReadIndex
	"github.com/kanon1343/fsegit/util"   // For FindGitRoot
	"github.com/spf13/cobra"
)

var showStage bool // Flag for --stage

// lsFilesCmd represents the ls-files command
var lsFilesCmd = &cobra.Command{
	Use:   "ls-files",
	Short: "Show information about files in the index and the working tree",
	Long: `This command shows information about files in the index.
With --stage, it shows staged content's mode, object name and stage number.`,
	Run: func(cmd *cobra.Command, args []string) {
		gitDir, err := util.FindGitRoot(".")
		if err != nil {
			log.Fatalf("fatal: not a git repository (or any of the parent directories): .git")
		}
		dotGitPath := filepath.Join(gitDir, ".git")

		idx, err := store.ReadIndex(dotGitPath)
		if err != nil {
			log.Fatalf("Failed to read index: %v", err)
		}

		if len(idx.Entries) == 0 {
			// fmt.Println("Index is empty.") // Or just print nothing
			return
		}

		for _, entry := range idx.Entries {
			if showStage {
				// Format: <mode_octal> <sha1_hash> <stage_number>	<path>
				// Stage number is in the higher bits of entry.Flags.
				// store.IndexEntry.Flags field should be public.
				stage := (entry.Flags >> 12) & 0x3 // Extract stage from bits 12 and 13
				fmt.Printf("%06o %s %d	%s\n", entry.Mode, entry.Hash.String(), stage, entry.PathName)
			} else {
				fmt.Println(entry.PathName)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(lsFilesCmd)
	lsFilesCmd.Flags().BoolVar(&showStage, "stage", false, "Show staged contents' mode, object name and stage number")
}
