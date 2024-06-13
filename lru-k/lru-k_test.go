package lru_k

import (
	"fmt"
	"testing"
)

type String string

func (d String) Len() int {
	return len(d)
}

func TestGet(t *testing.T) {
	lru := NewCache(10, int64(100))
	lru.Set("key1", String("1234"))
	if v, ok := lru.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	if _, ok := lru.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

func TestRemoveoldest(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	cap := len(k1 + k2 + v1 + v2)
	lru := NewCache(2, int64(cap))
	lru.Set(k1, String(v1))
	lru.Set(k2, String(v2))
	lru.Set(k3, String(v3))

	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("Removeoldest key1 failed")
	}
}

func TestEliminate(t *testing.T) {
	OnEliminateKeys := make([]string, 0)
	OnEliminateFun := func(key string, value any) {
		OnEliminateKeys = append(OnEliminateKeys, key)
	}

	maxitems := 10
	lru := NewCache(2, int64(maxitems*4), WithOnEliminate(OnEliminateFun))
	for i := 0; i < maxitems; i++ {
		lru.Set(fmt.Sprintf("m%d", i), String("11"))
	}

	// visit more than two times
	for i := 0; i < 10; i++ {
		lru.Get("m1")
		lru.Get("m0")
	}

	for i := maxitems; i < 12; i++ {
		lru.Set(fmt.Sprintf("m%d", i), String("22"))
	}

	//fmt.Println(lru)

	if len(OnEliminateKeys) != 2 {
		t.Fatalf("got %d evicted keys; want 2", len(OnEliminateKeys))
	}
	if OnEliminateKeys[0] != "m2" {
		t.Fatalf("got %v in first evicted key; want %s", OnEliminateKeys[0], "m2")
	}
	if OnEliminateKeys[1] != "m3" {
		t.Fatalf("got %v in second evicted key; want %s", OnEliminateKeys[1], "m3")
	}
}

func TestAdd(t *testing.T) {
	lru := NewCache(2, int64(6))
	lru.Set("key", String("1"))
	lru.Set("key", String("111"))

	if lru.BytesUsed() != int64(len("key")+len("111")) {
		t.Fatal("expected 6 but got", lru.BytesUsed())
	}
}
