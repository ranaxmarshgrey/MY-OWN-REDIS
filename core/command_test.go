package core

import (
	"bytes"
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

func TestStoreNaiveGet(t *testing.T) {
	clearStore()

	// Non-existent key returns nil
	if nonExistent := Get("no_such_key"); nonExistent != nil {
		t.Errorf("expected nil for non-existent key, got %v", nonExistent)
	}

	// Put and Get an object
	sampleObj := NewObject("temporary_val", 20)
	Put("expire_key", sampleObj)

	// Naive Get returns the exact pointer immediately
	got := Get("expire_key")
	if got != sampleObj {
		t.Fatalf("expected Get to return the exact pointer, got: %v", got)
	}
	if got.Value != "temporary_val" {
		t.Errorf("expected Value 'temporary_val', got: %v", got.Value)
	}

	// Sleep past the expiration time
	time.Sleep(30 * time.Millisecond)

	// Naive Get does NOT delete or filter out expired keys
	gotAfterExpire := Get("expire_key")
	if gotAfterExpire == nil {
		t.Fatalf("expected naive Get to return the object without checking expiration")
	}
	if gotAfterExpire.Value != "temporary_val" {
		t.Errorf("expected Value 'temporary_val', got: %v", gotAfterExpire.Value)
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


