package core

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func clearStore() {
	store = make(map[string]*Object)
}

func TestEvalSetBasic(t *testing.T) {
	clearStore()

	// Standard SET command
	resp, err := Eval([]string{"SET", "key1", "val1"})
	if err != nil {
		t.Fatalf("unexpected error for SET key1 val1: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}

	obj := Get("key1")
	if obj == nil {
		t.Fatalf("expected key1 to exist in store")
	}
	if obj.Value != "val1" {
		t.Errorf("expected val1, got %v", obj.Value)
	}
	if obj.ExpiresAt != -1 {
		t.Errorf("expected ExpiresAt -1, got %d", obj.ExpiresAt)
	}

	// Lowercase 'set' command
	resp, err = Eval([]string{"set", "key2", "val2"})
	if err != nil {
		t.Fatalf("unexpected error for lowercase set: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}

	obj = Get("key2")
	if obj == nil || obj.Value != "val2" {
		t.Errorf("expected val2 for key2, got %v", obj)
	}
}

func TestEvalSetWithEX(t *testing.T) {
	clearStore()

	startMs := time.Now().UnixMilli()
	resp, err := Eval([]string{"SET", "session", "tok123", "EX", "10"})
	if err != nil {
		t.Fatalf("unexpected error for SET with EX: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}

	obj := Get("session")
	if obj == nil {
		t.Fatalf("expected session to exist in store")
	}
	if obj.Value != "tok123" {
		t.Errorf("expected tok123, got %v", obj.Value)
	}

	expectedMin := startMs + 10*1000
	expectedMax := time.Now().UnixMilli() + 10*1000
	if obj.ExpiresAt < expectedMin || obj.ExpiresAt > expectedMax {
		t.Errorf("ExpiresAt %d not within expected window [%d, %d]", obj.ExpiresAt, expectedMin, expectedMax)
	}

	// Lowercase 'ex'
	resp, err = Eval([]string{"SET", "session_lower", "tok456", "ex", "5"})
	if err != nil {
		t.Fatalf("unexpected error for lowercase ex: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}
	obj = Get("session_lower")
	if obj == nil || obj.Value != "tok456" {
		t.Errorf("expected tok456 in store, got %v", obj)
	}
}

func TestEvalSetWithPX(t *testing.T) {
	clearStore()

	startMs := time.Now().UnixMilli()
	resp, err := Eval([]string{"SET", "cache_key", "data", "PX", "2500"})
	if err != nil {
		t.Fatalf("unexpected error for SET with PX: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}

	obj := Get("cache_key")
	if obj == nil {
		t.Fatalf("expected cache_key to exist in store")
	}
	if obj.Value != "data" {
		t.Errorf("expected data, got %v", obj.Value)
	}

	expectedMin := startMs + 2500
	expectedMax := time.Now().UnixMilli() + 2500
	if obj.ExpiresAt < expectedMin || obj.ExpiresAt > expectedMax {
		t.Errorf("ExpiresAt %d not within expected window [%d, %d]", obj.ExpiresAt, expectedMin, expectedMax)
	}

	// Lowercase 'px'
	resp, err = Eval([]string{"SET", "cache_key2", "data2", "px", "1000"})
	if err != nil {
		t.Fatalf("unexpected error for lowercase px: %v", err)
	}
	if !bytes.Equal(resp, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got %q", string(resp))
	}
}

func TestEvalSetOverwrite(t *testing.T) {
	clearStore()

	// Set with expiration first
	_, err := Eval([]string{"SET", "mykey", "val1", "EX", "60"})
	if err != nil {
		t.Fatalf("SET val1 failed: %v", err)
	}
	obj := Get("mykey")
	if obj == nil || obj.ExpiresAt == -1 {
		t.Fatalf("expected mykey to have expiration set")
	}

	// Overwrite without expiration
	_, err = Eval([]string{"SET", "mykey", "val2"})
	if err != nil {
		t.Fatalf("SET val2 overwrite failed: %v", err)
	}
	obj = Get("mykey")
	if obj == nil {
		t.Fatalf("expected mykey to exist")
	}
	if obj.Value != "val2" {
		t.Errorf("expected updated value 'val2', got %v", obj.Value)
	}
	if obj.ExpiresAt != -1 {
		t.Errorf("expected TTL to be cleared (ExpiresAt -1), got %d", obj.ExpiresAt)
	}
}

func TestEvalSetErrors(t *testing.T) {
	clearStore()

	tests := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		// Wrong number of arguments
		{"No args", []string{"SET"}, "ERR wrong number of arguments"},
		{"Only key", []string{"SET", "key"}, "ERR wrong number of arguments"},

		// Syntax errors
		{"Unknown option", []string{"SET", "k", "v", "FOOBAR"}, "ERR syntax error"},
		{"EX missing value", []string{"SET", "k", "v", "EX"}, "ERR syntax error"},
		{"PX missing value", []string{"SET", "k", "v", "PX"}, "ERR syntax error"},
		{"Trailing extra argument", []string{"SET", "k", "v", "EX", "10", "EXTRA"}, "ERR syntax error"},

		// Invalid expire time
		{"EX not a number", []string{"SET", "k", "v", "EX", "not_a_num"}, "ERR invalid expire time"},
		{"EX zero", []string{"SET", "k", "v", "EX", "0"}, "ERR invalid expire time"},
		{"EX negative", []string{"SET", "k", "v", "EX", "-5"}, "ERR invalid expire time"},
		{"PX not a number", []string{"SET", "k", "v", "PX", "not_a_num"}, "ERR invalid expire time"},
		{"PX zero", []string{"SET", "k", "v", "PX", "0"}, "ERR invalid expire time"},
		{"PX negative", []string{"SET", "k", "v", "PX", "-100"}, "ERR invalid expire time"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Eval(tc.args)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.expectedErr)
			}
			if err.Error() != tc.expectedErr {
				t.Errorf("expected error %q, got %q", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestStorePassiveGet(t *testing.T) {
	clearStore()

	// Non-existent key returns nil
	if nonExistent := Get("no_such_key"); nonExistent != nil {
		t.Errorf("expected nil for non-existent key, got %v", nonExistent)
	}

	// Put and Get an object with expiration
	sampleObj := NewObject("temporary_val", 20)
	Put("expire_key", sampleObj)

	// Get returns the exact pointer before expiration
	got := Get("expire_key")
	if got != sampleObj {
		t.Fatalf("expected Get to return the exact pointer, got: %v", got)
	}
	if got.Value != "temporary_val" {
		t.Errorf("expected Value 'temporary_val', got: %v", got.Value)
	}

	// Sleep past the expiration time
	time.Sleep(30 * time.Millisecond)

	// Passive Get deletes expired keys and returns nil
	gotAfterExpire := Get("expire_key")
	if gotAfterExpire != nil {
		t.Fatalf("expected Get to return nil for expired key, got: %v", gotAfterExpire)
	}

	// Verify key was removed from the store map
	if _, exists := store["expire_key"]; exists {
		t.Errorf("expected expired key 'expire_key' to be deleted from store map")
	}
}

func TestStoreDelete(t *testing.T) {
	clearStore()

	// 1. Delete non-existent key returns false
	if deleted := Delete("no_such_key"); deleted {
		t.Errorf("expected Delete to return false for non-existent key")
	}

	// 2. Put and Delete existing key returns true
	Put("del_key", NewObject("del_val", -1))
	if deleted := Delete("del_key"); !deleted {
		t.Errorf("expected Delete to return true for existing key")
	}

	// 3. Verify key is gone
	if got := Get("del_key"); got != nil {
		t.Errorf("expected del_key to be nil after deletion, got %v", got)
	}
}


func TestEvalGet(t *testing.T) {
	clearStore()

	// 1. GET non-existent key -> returns $-1\r\n (nil bulk string)
	resp, err := Eval([]string{"GET", "missing"})
	if err != nil {
		t.Fatalf("unexpected error for GET missing: %v", err)
	}
	if !bytes.Equal(resp, []byte("$-1\r\n")) {
		t.Fatalf("expected $-1\\r\\n for missing key, got %q", string(resp))
	}

	// 2. SET then GET key
	_, err = Eval([]string{"SET", "name", "redis"})
	if err != nil {
		t.Fatalf("unexpected error for SET: %v", err)
	}
	resp, err = Eval([]string{"GET", "name"})
	if err != nil {
		t.Fatalf("unexpected error for GET name: %v", err)
	}
	if !bytes.Equal(resp, []byte("$5\r\nredis\r\n")) {
		t.Fatalf("expected $5\\r\\nredis\\r\\n, got %q", string(resp))
	}

	// Case-insensitivity: 'get'
	resp, err = Eval([]string{"get", "name"})
	if err != nil {
		t.Fatalf("unexpected error for lowercase get: %v", err)
	}
	if !bytes.Equal(resp, []byte("$5\r\nredis\r\n")) {
		t.Fatalf("expected $5\\r\\nredis\\r\\n, got %q", string(resp))
	}

	// 3. GET on expired key -> returns $-1\r\n and deletes from store
	_, err = Eval([]string{"SET", "short_ttl", "val", "PX", "20"})
	if err != nil {
		t.Fatalf("unexpected error for SET PX: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	resp, err = Eval([]string{"GET", "short_ttl"})
	if err != nil {
		t.Fatalf("unexpected error for GET expired key: %v", err)
	}
	if !bytes.Equal(resp, []byte("$-1\r\n")) {
		t.Fatalf("expected $-1\\r\\n for expired key, got %q", string(resp))
	}
	// Verify key was deleted from store by EvalGet
	if _, exists := store["short_ttl"]; exists {
		t.Errorf("expected expired key 'short_ttl' to be removed from store by EvalGet")
	}

	// 4. Argument validation
	badArgs := [][]string{
		{"GET"},
		{"GET", "k1", "k2"},
	}
	for _, args := range badArgs {
		_, err := Eval(args)
		if err == nil {
			t.Fatalf("expected error for %v, got nil", args)
		}
	}
}

func TestEvalTtl(t *testing.T) {
	clearStore()

	// 1. TTL on non-existent key -> returns :-2\r\n
	resp, err := Eval([]string{"TTL", "missing_key"})
	if err != nil {
		t.Fatalf("unexpected error for TTL missing_key: %v", err)
	}
	if !bytes.Equal(resp, []byte(":-2\r\n")) {
		t.Fatalf("expected :-2\\r\\n, got %q", string(resp))
	}

	// 2. TTL on key without expiration -> returns :-1\r\n
	_, err = Eval([]string{"SET", "persist_key", "hello"})
	if err != nil {
		t.Fatalf("unexpected error for SET: %v", err)
	}
	resp, err = Eval([]string{"TTL", "persist_key"})
	if err != nil {
		t.Fatalf("unexpected error for TTL persist_key: %v", err)
	}
	if !bytes.Equal(resp, []byte(":-1\r\n")) {
		t.Fatalf("expected :-1\\r\\n, got %q", string(resp))
	}

	// 3. TTL on key with EX expiration (100 seconds)
	_, err = Eval([]string{"SET", "expiring_key", "val", "EX", "100"})
	if err != nil {
		t.Fatalf("unexpected error for SET with EX: %v", err)
	}
	resp, err = Eval([]string{"TTL", "expiring_key"})
	if err != nil {
		t.Fatalf("unexpected error for TTL expiring_key: %v", err)
	}
	// The TTL should be around 99 or 100
	if !bytes.Equal(resp, []byte(":100\r\n")) && !bytes.Equal(resp, []byte(":99\r\n")) {
		t.Fatalf("expected :100\\r\\n or :99\\r\\n, got %q", string(resp))
	}

	// Case-insensitivity: lowercase 'ttl'
	resp, err = Eval([]string{"ttl", "expiring_key"})
	if err != nil {
		t.Fatalf("unexpected error for lowercase ttl: %v", err)
	}
	if !bytes.Equal(resp, []byte(":100\r\n")) && !bytes.Equal(resp, []byte(":99\r\n")) {
		t.Fatalf("expected :100\\r\\n or :99\\r\\n, got %q", string(resp))
	}

	// 4. TTL on key with PX expiration (2500 ms -> 2 seconds)
	_, err = Eval([]string{"SET", "px_key", "val", "PX", "2500"})
	if err != nil {
		t.Fatalf("unexpected error for SET with PX: %v", err)
	}
	resp, err = Eval([]string{"TTL", "px_key"})
	if err != nil {
		t.Fatalf("unexpected error for TTL px_key: %v", err)
	}
	if !bytes.Equal(resp, []byte(":2\r\n")) {
		t.Fatalf("expected :2\\r\\n, got %q", string(resp))
	}

	// 5. TTL on expired key -> returns :-2\r\n and removes from store
	_, err = Eval([]string{"SET", "quick_expire", "val", "PX", "20"})
	if err != nil {
		t.Fatalf("unexpected error for SET with PX: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	resp, err = Eval([]string{"TTL", "quick_expire"})
	if err != nil {
		t.Fatalf("unexpected error for TTL on expired key: %v", err)
	}
	if !bytes.Equal(resp, []byte(":-2\r\n")) {
		t.Fatalf("expected :-2\\r\\n for expired key, got %q", string(resp))
	}
	// Verify key was deleted by EvalTtl
	if _, exists := store["quick_expire"]; exists {
		t.Errorf("expected expired key 'quick_expire' to be removed from store by EvalTtl")
	}

	// 6. Argument validation
	badArgs := [][]string{
		{"TTL"},
		{"TTL", "k1", "k2"},
	}
	for _, args := range badArgs {
		_, err := Eval(args)
		if err == nil {
			t.Fatalf("expected error for %v, got nil", args)
		}
	}
}

func TestEvalDel(t *testing.T) {
	clearStore()

	// 1. DEL non-existent key returns :0\r\n
	resp, err := Eval([]string{"DEL", "non_existent"})
	if err != nil {
		t.Fatalf("unexpected error for DEL non_existent: %v", err)
	}
	if !bytes.Equal(resp, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n, got %q", string(resp))
	}

	// 2. DEL single existing key returns :1\r\n and removes it
	_, err = Eval([]string{"SET", "del_k1", "val1"})
	if err != nil {
		t.Fatalf("SET del_k1 failed: %v", err)
	}
	resp, err = Eval([]string{"DEL", "del_k1"})
	if err != nil {
		t.Fatalf("DEL del_k1 failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n, got %q", string(resp))
	}
	if got := Get("del_k1"); got != nil {
		t.Errorf("expected del_k1 to be nil after deletion")
	}

	// 3. Multi-key DEL: all keys exist
	_, _ = Eval([]string{"SET", "m1", "v1"})
	_, _ = Eval([]string{"SET", "m2", "v2"})
	_, _ = Eval([]string{"SET", "m3", "v3"})
	resp, err = Eval([]string{"DEL", "m1", "m2", "m3"})
	if err != nil {
		t.Fatalf("multi-key DEL failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":3\r\n")) {
		t.Fatalf("expected :3\\r\\n for deleting 3 keys, got %q", string(resp))
	}
	for _, k := range []string{"m1", "m2", "m3"} {
		if got := Get(k); got != nil {
			t.Errorf("expected %s to be deleted", k)
		}
	}

	// 4. Multi-key DEL: some exist, some don't
	_, _ = Eval([]string{"SET", "exist1", "v1"})
	_, _ = Eval([]string{"SET", "exist2", "v2"})
	resp, err = Eval([]string{"DEL", "exist1", "missing_k", "exist2"})
	if err != nil {
		t.Fatalf("mixed DEL failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":2\r\n")) {
		t.Fatalf("expected :2\\r\\n for 2 existing keys, got %q", string(resp))
	}

	// 5. Duplicate keys in DEL argument list: count reflects unique deletes
	_, _ = Eval([]string{"SET", "dup_k", "val"})
	resp, err = Eval([]string{"DEL", "dup_k", "dup_k"})
	if err != nil {
		t.Fatalf("duplicate DEL failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n for duplicate key in single DEL, got %q", string(resp))
	}

	// 6. DEL on already expired key returns :0\r\n and removes from store
	_, _ = Eval([]string{"SET", "exp_del_key", "val", "PX", "20"})
	time.Sleep(30 * time.Millisecond)
	resp, err = Eval([]string{"DEL", "exp_del_key"})
	if err != nil {
		t.Fatalf("DEL expired key failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n for expired key DEL, got %q", string(resp))
	}
	if _, exists := store["exp_del_key"]; exists {
		t.Errorf("expected expired key 'exp_del_key' to be deleted from store map")
	}

	// 7. Case-insensitivity: lowercase 'del'
	_, _ = Eval([]string{"SET", "lower_k", "val"})
	resp, err = Eval([]string{"del", "lower_k"})
	if err != nil {
		t.Fatalf("lowercase del failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n for lowercase del, got %q", string(resp))
	}

	// 8. Argument error validation
	_, err = Eval([]string{"DEL"})
	if err == nil {
		t.Fatalf("expected error for DEL without arguments, got nil")
	}
	if err.Error() != "ERR wrong number of arguments for 'del' command" {
		t.Errorf("expected error %q, got %q", "ERR wrong number of arguments for 'del' command", err.Error())
	}
}

func TestEvalExpire(t *testing.T) {
	clearStore()

	// 1. EXPIRE on non-existent key returns :0\r\n
	resp, err := Eval([]string{"EXPIRE", "no_key", "10"})
	if err != nil {
		t.Fatalf("unexpected error for EXPIRE on missing key: %v", err)
	}
	if !bytes.Equal(resp, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n for missing key, got %q", string(resp))
	}

	// 2. EXPIRE on existing persistent key returns :1\r\n and sets TTL
	_, _ = Eval([]string{"SET", "session_user", "alice"})
	resp, err = Eval([]string{"EXPIRE", "session_user", "10"})
	if err != nil {
		t.Fatalf("EXPIRE failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n, got %q", string(resp))
	}

	ttlResp, err := Eval([]string{"TTL", "session_user"})
	if err != nil {
		t.Fatalf("TTL failed: %v", err)
	}
	if !bytes.Equal(ttlResp, []byte(":10\r\n")) && !bytes.Equal(ttlResp, []byte(":9\r\n")) {
		t.Fatalf("expected :10\\r\\n or :9\\r\\n, got %q", string(ttlResp))
	}

	// 3. EXPIRE overwriting existing expiration
	resp, err = Eval([]string{"EXPIRE", "session_user", "100"})
	if err != nil {
		t.Fatalf("EXPIRE overwrite failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n, got %q", string(resp))
	}
	ttlResp, _ = Eval([]string{"TTL", "session_user"})
	if !bytes.Equal(ttlResp, []byte(":100\r\n")) && !bytes.Equal(ttlResp, []byte(":99\r\n")) {
		t.Fatalf("expected updated TTL :100\\r\\n or :99\\r\\n, got %q", string(ttlResp))
	}

	// 4. EXPIRE on already expired key returns :0\r\n and cleans up
	_, _ = Eval([]string{"SET", "quick_key", "data", "PX", "20"})
	time.Sleep(30 * time.Millisecond)
	resp, err = Eval([]string{"EXPIRE", "quick_key", "10"})
	if err != nil {
		t.Fatalf("EXPIRE on expired key failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n for already expired key, got %q", string(resp))
	}
	if _, exists := store["quick_key"]; exists {
		t.Errorf("expected expired key 'quick_key' to be cleaned up from store map")
	}

	// 5. Case-insensitivity: lowercase 'expire'
	_, _ = Eval([]string{"SET", "case_exp", "val"})
	resp, err = Eval([]string{"expire", "case_exp", "50"})
	if err != nil {
		t.Fatalf("lowercase expire failed: %v", err)
	}
	if !bytes.Equal(resp, []byte(":1\r\n")) {
		t.Fatalf("expected :1\\r\\n, got %q", string(resp))
	}

	// 6. Argument error validation
	badTests := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{"No args", []string{"EXPIRE"}, "ERR wrong number of arguments for 'expire' command"},
		{"1 arg only", []string{"EXPIRE", "key"}, "ERR wrong number of arguments for 'expire' command"},
		{"3 args (extra)", []string{"EXPIRE", "key", "10", "EXTRA"}, "ERR wrong number of arguments for 'expire' command"},
		{"Not a number", []string{"EXPIRE", "key", "abc"}, "ERR invalid expire time"},
		{"Zero seconds", []string{"EXPIRE", "key", "0"}, "ERR invalid expire time"},
		{"Negative seconds", []string{"EXPIRE", "key", "-10"}, "ERR invalid expire time"},
	}
	for _, tc := range badTests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Eval(tc.args)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.expectedErr)
			}
			if err.Error() != tc.expectedErr {
				t.Errorf("expected error %q, got %q", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestPassiveCleanupComprehensive(t *testing.T) {
	clearStore()

	// Set 4 keys with a short TTL (20ms)
	keys := []string{"passive_get", "passive_ttl", "passive_expire", "passive_del"}
	for _, k := range keys {
		Put(k, NewObject("val", 20))
	}

	// Wait past expiration
	time.Sleep(30 * time.Millisecond)

	// Ensure all 4 keys are still physically in the store map before access
	for _, k := range keys {
		if _, exists := store[k]; !exists {
			t.Fatalf("expected key %s to still be in store map before access", k)
		}
	}

	// 1. Passive cleanup via GET
	getResp, err := Eval([]string{"GET", "passive_get"})
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if !bytes.Equal(getResp, []byte("$-1\r\n")) {
		t.Errorf("expected $-1\\r\\n, got %q", string(getResp))
	}
	if _, exists := store["passive_get"]; exists {
		t.Errorf("expected 'passive_get' to be removed from store map after GET")
	}

	// 2. Passive cleanup via TTL
	ttlResp, err := Eval([]string{"TTL", "passive_ttl"})
	if err != nil {
		t.Fatalf("TTL failed: %v", err)
	}
	if !bytes.Equal(ttlResp, []byte(":-2\r\n")) {
		t.Errorf("expected :-2\\r\\n, got %q", string(ttlResp))
	}
	if _, exists := store["passive_ttl"]; exists {
		t.Errorf("expected 'passive_ttl' to be removed from store map after TTL")
	}

	// 3. Passive cleanup via EXPIRE
	expResp, err := Eval([]string{"EXPIRE", "passive_expire", "10"})
	if err != nil {
		t.Fatalf("EXPIRE failed: %v", err)
	}
	if !bytes.Equal(expResp, []byte(":0\r\n")) {
		t.Errorf("expected :0\\r\\n, got %q", string(expResp))
	}
	if _, exists := store["passive_expire"]; exists {
		t.Errorf("expected 'passive_expire' to be removed from store map after EXPIRE")
	}

	// 4. Passive cleanup via DEL
	delResp, err := Eval([]string{"DEL", "passive_del"})
	if err != nil {
		t.Fatalf("DEL failed: %v", err)
	}
	if !bytes.Equal(delResp, []byte(":0\r\n")) {
		t.Errorf("expected :0\\r\\n, got %q", string(delResp))
	}
	if _, exists := store["passive_del"]; exists {
		t.Errorf("expected 'passive_del' to be removed from store map after DEL")
	}
}

func TestDeleteExpiredKeys(t *testing.T) {
	oldLimit := keyLimit
	keyLimit = 100
	defer func() { keyLimit = oldLimit }()

	// 1. Empty store: should return gracefully without panic
	clearStore()
	DeleteExpiredKeys()
	if len(store) != 0 {
		t.Fatalf("expected store to remain empty, got len %d", len(store))
	}

	// 2. Store with only persistent keys (-1): none should be deleted
	clearStore()
	for i := 0; i < 25; i++ {
		Put(fmt.Sprintf("persist_%d", i), NewObject("val", -1))
	}
	DeleteExpiredKeys()
	if len(store) != 25 {
		t.Fatalf("expected 25 persistent keys to remain, got %d", len(store))
	}

	// 3. Store with only unexpired future keys: none should be deleted
	clearStore()
	for i := 0; i < 25; i++ {
		Put(fmt.Sprintf("future_%d", i), NewObject("val", 100000))
	}
	DeleteExpiredKeys()
	if len(store) != 25 {
		t.Fatalf("expected 25 future keys to remain, got %d", len(store))
	}

	// 4. Mixed store: persistent keys, future keys, and expired keys
	clearStore()
	for i := 0; i < 5; i++ {
		Put(fmt.Sprintf("keep_persist_%d", i), NewObject("val", -1))
	}
	for i := 0; i < 5; i++ {
		Put(fmt.Sprintf("keep_future_%d", i), NewObject("val", 100000))
	}
	for i := 0; i < 10; i++ {
		Put(fmt.Sprintf("expired_%d", i), NewObject("val", 15))
	}
	time.Sleep(25 * time.Millisecond)

	DeleteExpiredKeys()

	// All 10 expired keys should be deleted
	for i := 0; i < 10; i++ {
		if _, exists := store[fmt.Sprintf("expired_%d", i)]; exists {
			t.Errorf("expected expired_%d to be deleted by active cleanup", i)
		}
	}
	// All 5 persistent + 5 future keys must remain
	for i := 0; i < 5; i++ {
		if _, exists := store[fmt.Sprintf("keep_persist_%d", i)]; !exists {
			t.Errorf("expected keep_persist_%d to remain", i)
		}
		if _, exists := store[fmt.Sprintf("keep_future_%d", i)]; !exists {
			t.Errorf("expected keep_future_%d to remain", i)
		}
	}
	if len(store) != 10 {
		t.Fatalf("expected 10 remaining keys, got %d", len(store))
	}

	// 5. Multi-cycle sampling (>20 expired keys)
	// Tests that the loop continues sampling when expiredFraction > 0.25
	clearStore()
	const numExpired = 60
	for i := 0; i < numExpired; i++ {
		Put(fmt.Sprintf("batch_exp_%d", i), NewObject("val", 15))
	}
	for i := 0; i < 5; i++ {
		Put(fmt.Sprintf("batch_persist_%d", i), NewObject("val", -1))
	}
	time.Sleep(25 * time.Millisecond)

	DeleteExpiredKeys()

	// Multi-cycle cleanup should have purged all 60 expired keys
	for i := 0; i < numExpired; i++ {
		if _, exists := store[fmt.Sprintf("batch_exp_%d", i)]; exists {
			t.Errorf("expected batch_exp_%d to be cleaned up in multi-cycle cleanup", i)
		}
	}
	if len(store) != 5 {
		t.Fatalf("expected 5 persistent keys remaining, got %d", len(store))
	}

	// 6. Stop condition: sample has <= 25% expired keys
	clearStore()
	// Put 3 expired keys (15% of 20 sampled) and 17 active keys
	for i := 0; i < 3; i++ {
		Put(fmt.Sprintf("low_exp_%d", i), NewObject("val", 15))
	}
	for i := 0; i < 17; i++ {
		Put(fmt.Sprintf("active_%d", i), NewObject("val", 100000))
	}
	time.Sleep(25 * time.Millisecond)

	DeleteExpiredKeys()

	// The 17 active keys must still exist
	for i := 0; i < 17; i++ {
		if _, exists := store[fmt.Sprintf("active_%d", i)]; !exists {
			t.Errorf("expected active_%d to still exist", i)
		}
	}
}



