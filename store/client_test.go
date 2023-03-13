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
	hashString := "0007cdd154bfd7fa617fe6a0e18685682856f16c"
	hash, err := hex.DecodeString(hashString)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := client.GetObject(hash)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(obj.Data))
}
