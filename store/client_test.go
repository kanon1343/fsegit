package store

import (
	"encoding/hex"
	"testing"
)

// コミットオブジェクトが正しく取れるか
func TestClient_GetObject(t *testing.T) {
	client, err := NewClient("/Users/haradakanon/Desktop/Atcoder")
	if err != nil {
		t.Fatal(err)
	}
	hashString := "366fa17c32ca232790db770d4e37898e48bdd2ce"
	hash, err := hex.DecodeString(hashString)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := client.GetObject(hash)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(obj.Type))
}
