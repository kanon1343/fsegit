package cmd

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/spf13/cobra"
)

// logCmd represents the log command
var logCmd = &cobra.Command{
	Use:   "log",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 最新のコミットオブジェクトを取得.
		f, err := os.Open("./.git/HEAD")
		if err != nil {
			log.Fatal()
		}
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {

			}
		}(f)
		buf, err := ioutil.ReadAll(f)
		if err != nil {
			log.Fatal(err)
		}
		head := string(buf)
		headLength := len(head) - 1
		latestCommitHash := filepath.Join(".git/", head[5:headLength])
		f, err = os.Open(latestCommitHash)
		if err != nil {
			log.Fatal(err)
		}
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {

			}
		}(f)
		buf, err = ioutil.ReadAll(f)
		headFilePath := string(buf)
		hash, err := hex.DecodeString(headFilePath[:40])
		if err != nil {
			log.Fatal(err)
		}

		// コミット履歴を探索し、出力.
		client, err := store.NewClient("./")
		if err != nil {
			log.Fatal()
		}
		if err := client.WalkHistory(hash, func(commit *object.Commit) error {
			fmt.Println(commit)
			fmt.Println("")
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(logCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// logCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// logCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
