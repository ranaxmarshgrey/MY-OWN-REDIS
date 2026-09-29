package core

import (
	"os"
	"testing"
	"time"
)

// cleanAOF removes any AOF artefacts left by a previous test run.
func cleanAOF(t *testing.T) {
	t.Helper()
	for _, f := range []string{AOFFile, AOFFileTmp} {
		_ = os.Remove(f)
	}
}

// TestDumpAndLoadAOF verifies the core round-trip: put keys → dump → clear store → load → all keys restored.
func TestDumpAndLoadAOF(t *testing.T) {
	ClearStore()
	cleanAOF(t)
	t.Cleanup(func() { cleanAOF(t) })

	// Use an unlimited store for this test.
	origLimit := GetKeyLimit()
	SetKeyLimit(0)
	t.Cleanup(func() { SetKeyLimit(origLimit) })

	// Populate some keys (no TTL).
	Put("hello", NewObject("world", -1))
	Put("foo", NewObject("bar", -1))
	Put("num", NewObject("42", -1))

	if err := DumpAllAOF(); err != nil {
		t.Fatalf("DumpAllAOF: %v", err)
	}

	// Wipe store and reload from file.
	ClearStore()

	n, err := LoadAOF()
	if err != nil {
		t.Fatalf("LoadAOF: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 commands replayed, got %d", n)
	}

	checks := map[string]string{
		"hello": "world",
		"foo":   "bar",
		"num":   "42",
	}
	for key, want := range checks {
		obj := Get(key)
		if obj == nil {
			t.Errorf("key %q missing after AOF load", key)
			continue
		}
		if got := obj.Value.(string); got != want {
			t.Errorf("key %q: want %q, got %q", key, want, got)
		}
	}
}

// TestDumpAOFSkipsExpiredKeys checks that keys already past their TTL are
// not written to the AOF file.
func TestDumpAOFSkipsExpiredKeys(t *testing.T) {
	ClearStore()
	cleanAOF(t)
	t.Cleanup(func() { cleanAOF(t) })

	origLimit := GetKeyLimit()
	SetKeyLimit(0)
	t.Cleanup(func() { SetKeyLimit(origLimit) })

	// A key that expires in 1 hour (should be written).
	Put("alive", NewObject("yes", 3600000))
	// A key that is already expired (simulate by setting ExpiresAt in the past).
	expired := NewObject("no", -1)
	expired.ExpiresAt = time.Now().UnixMilli() - 1000
	Put("dead", expired)

	if err := DumpAllAOF(); err != nil {
		t.Fatalf("DumpAllAOF: %v", err)
	}

	ClearStore()
	n, err := LoadAOF()
	if err != nil {
		t.Fatalf("LoadAOF: %v", err)
	}
	// Only "alive" → SET + PEXPIREAT = 2 commands; "dead" should be skipped.
	if n != 2 {
		t.Errorf("expected 2 commands replayed (SET + PEXPIREAT for alive), got %d", n)
	}
	if Get("dead") != nil {
		t.Error("expired key 'dead' should not be present after AOF load")
	}
	if Get("alive") == nil {
		t.Error("key 'alive' should be present after AOF load")
	}
}

// TestLoadAOFNoFile verifies that LoadAOF is a no-op (not an error) when
// no AOF file exists yet.
func TestLoadAOFNoFile(t *testing.T) {
	cleanAOF(t)
	t.Cleanup(func() { cleanAOF(t) })

	n, err := LoadAOF()
	if err != nil {
		t.Fatalf("LoadAOF on missing file should not error, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 replayed, got %d", n)
	}
}

// TestBgRewriteAOFCmd exercises the BGREWRITEAOF command handler end-to-end.
func TestBgRewriteAOFCmd(t *testing.T) {
	ClearStore()
	cleanAOF(t)
	t.Cleanup(func() { cleanAOF(t) })

	origLimit := GetKeyLimit()
	SetKeyLimit(0)
	t.Cleanup(func() { SetKeyLimit(origLimit) })

	Put("x", NewObject("1", -1))

	resp, err := Eval([]string{"BGREWRITEAOF"})
	if err != nil {
		t.Fatalf("BGREWRITEAOF returned error: %v", err)
	}
	want := "+Background append only file rewriting started\r\n"
	if string(resp) != want {
		t.Errorf("unexpected response: %q", resp)
	}

	// Give the background goroutine time to finish.
	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(AOFFile); os.IsNotExist(err) {
		t.Error("AOF file was not created after BGREWRITEAOF")
	}
}
