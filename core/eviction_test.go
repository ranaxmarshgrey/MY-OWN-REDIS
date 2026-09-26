package core

import (
	"bytes"
	"testing"
)

// TestEvictRandomBasic verifies evictRandom returns false on empty store
// and true when successfully removing a key.
func TestEvictRandomBasic(t *testing.T) {
	clearStore()

	// 1. Empty store: evictRandom should return false
	if evicted := evictRandom(); evicted {
		t.Fatalf("expected evictRandom on empty store to return false, got true")
	}

	// 2. Single key: evictRandom should remove the key and return true
	store["onlyKey"] = NewObject("val", -1)
	if evicted := evictRandom(); !evicted {
		t.Fatalf("expected evictRandom to return true when a key exists")
	}
	if len(store) != 0 {
		t.Fatalf("expected store to be empty after evicting only key, got len %d", len(store))
	}

	// 3. Multiple keys: evictRandom removes one key per invocation
	store["k1"] = NewObject("v1", -1)
	store["k2"] = NewObject("v2", -1)
	store["k3"] = NewObject("v3", -1)

	if !evictRandom() || len(store) != 2 {
		t.Fatalf("expected store len 2 after 1st eviction, got %d", len(store))
	}
	if !evictRandom() || len(store) != 1 {
		t.Fatalf("expected store len 1 after 2nd eviction, got %d", len(store))
	}
	if !evictRandom() || len(store) != 0 {
		t.Fatalf("expected store len 0 after 3rd eviction, got %d", len(store))
	}
	if evictRandom() {
		t.Fatalf("expected evictRandom on empty store to return false")
	}
}

// TestPutEvictionAtLimit tests Step 1 scenario:
// When store is at capacity (keyLimit = 5), inserting a 6th new key evicts one existing key.
func TestPutEvictionAtLimit(t *testing.T) {
	clearStore()
	SetKeyLimit(5)

	initialKeys := []string{"a", "b", "c", "d", "e"}
	for i, k := range initialKeys {
		Put(k, NewObject(i+1, -1))
	}

	if len(store) != 5 {
		t.Fatalf("expected store to have 5 keys, got %d", len(store))
	}

	// All 5 initial keys should exist
	for _, k := range initialKeys {
		if _, exists := store[k]; !exists {
			t.Fatalf("expected key %s to exist in store", k)
		}
	}

	// Insert 6th key: "f"
	Put("f", NewObject(6, -1))

	// Store size must remain capped at 5
	if len(store) != 5 {
		t.Fatalf("expected store length to remain 5, got %d", len(store))
	}

	// The newly inserted key "f" must exist
	fObj := Get("f")
	if fObj == nil || fObj.Value != 6 {
		t.Fatalf("expected key 'f' to exist with value 6, got %v", fObj)
	}

	// Exactly 4 of the 5 original keys should remain, and 1 evicted
	remainingCount := 0
	for _, k := range initialKeys {
		if _, exists := store[k]; exists {
			remainingCount++
		}
	}

	if remainingCount != 4 {
		t.Fatalf("expected exactly 4 original keys remaining, got %d", remainingCount)
	}
}

// TestPutOverwriteExistingKeyDoesNotEvict tests the critical correction:
// Overwriting an existing key when store is full must NOT evict any other key.
func TestPutOverwriteExistingKeyDoesNotEvict(t *testing.T) {
	clearStore()
	SetKeyLimit(5)

	initialKeys := []string{"a", "b", "c", "d", "e"}
	for i, k := range initialKeys {
		Put(k, NewObject(i+1, -1))
	}

	if len(store) != 5 {
		t.Fatalf("expected store length 5, got %d", len(store))
	}

	// Overwrite existing key "a" with NEW_VALUE
	Put("a", NewObject("NEW_VALUE", -1))

	// Store length must still be 5
	if len(store) != 5 {
		t.Fatalf("expected store length to remain 5, got %d", len(store))
	}

	// Key "a" must have updated value
	aObj := Get("a")
	if aObj == nil || aObj.Value != "NEW_VALUE" {
		t.Fatalf("expected key 'a' to have value 'NEW_VALUE', got %v", aObj)
	}

	// ALL other keys (b, c, d, e) must still exist - no eviction should have occurred!
	for _, k := range []string{"b", "c", "d", "e"} {
		if _, exists := store[k]; !exists {
			t.Fatalf("expected key %q to still exist; it was unexpectedly evicted during overwrite!", k)
		}
	}
}

// TestConfigurableKeyLimit verifies SetKeyLimit and GetKeyLimit behavior.
func TestConfigurableKeyLimit(t *testing.T) {
	clearStore()
	defer SetKeyLimit(DefaultKeyLimit)

	SetKeyLimit(2)
	if GetKeyLimit() != 2 {
		t.Fatalf("expected key limit 2, got %d", GetKeyLimit())
	}

	Put("x", NewObject(1, -1))
	Put("y", NewObject(2, -1))
	if len(store) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(store))
	}

	// Inserting 3rd key should evict one to stay at limit 2
	Put("z", NewObject(3, -1))
	if len(store) != 2 {
		t.Fatalf("expected store length to remain 2, got %d", len(store))
	}
	if Get("z") == nil {
		t.Fatalf("expected key 'z' to be in store")
	}

	// Increase limit to 4
	SetKeyLimit(4)
	Put("w", NewObject(4, -1))
	Put("v", NewObject(5, -1))
	if len(store) != 4 {
		t.Fatalf("expected store length to be 4 after increasing limit, got %d", len(store))
	}
}

// TestEvalSetWithEviction verifies end-to-end command evaluation with eviction.
func TestEvalSetWithEviction(t *testing.T) {
	clearStore()
	SetKeyLimit(5)

	// SET a..e
	for _, k := range []string{"k1", "k2", "k3", "k4", "k5"} {
		resp, err := Eval([]string{"SET", k, "v_" + k})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(resp, []byte("+OK\r\n")) {
			t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
		}
	}

	if len(store) != 5 {
		t.Fatalf("expected 5 keys, got %d", len(store))
	}

	// Overwrite k3
	resp, err := Eval([]string{"SET", "k3", "v3_updated"})
	if err != nil || !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("unexpected error or response for overwrite: %v, %s", err, string(resp))
	}
	if len(store) != 5 {
		t.Fatalf("expected store length 5 after overwrite, got %d", len(store))
	}
	for _, k := range []string{"k1", "k2", "k3", "k4", "k5"} {
		if Get(k) == nil {
			t.Fatalf("expected key %s to exist after overwrite", k)
		}
	}

	// Insert 6th key via SET
	resp, err = Eval([]string{"SET", "k6", "v6"})
	if err != nil || !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("unexpected error or response for 6th key: %v, %s", err, string(resp))
	}
	if len(store) != 5 {
		t.Fatalf("expected store length 5 after inserting 6th key, got %d", len(store))
	}
	if Get("k6") == nil {
		t.Fatalf("expected k6 to be present in store")
	}
}
