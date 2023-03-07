package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
)

func main() {
	// hashString := os.Args[1]
	hashString := "0007cdd154bfd7fa617fe6a0e18685682856f16c"
	hash, err := hex.DecodeString(hashString)
	if err != nil {
		log.Fatal(err)
	}

	client, err := store.NewClient("/Users/haradakanon/Desktop/Atcoder")
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
}
