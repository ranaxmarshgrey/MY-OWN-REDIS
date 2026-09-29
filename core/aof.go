package core

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const (
	AOFFile    = "appendonly.aof"
	AOFFileTmp = "appendonly.aof.tmp"
)

// encodeSetCmd RESP-encodes a SET command for the given key/value pair.
// If the object has a future expiration, a PEXPIREAT command is appended
// so the TTL is also preserved when the AOF is replayed.
//
// Format for SET:
//
//	*3\r\n$3\r\nSET\r\n$<klen>\r\n<key>\r\n$<vlen>\r\n<value>\r\n
//
// Format for PEXPIREAT (optional):
//
//	*3\r\n$10\r\nPEXPIREAT\r\n$<klen>\r\n<key>\r\n$<tslen>\r\n<ts>\r\n
func encodeSetCmd(key string, obj *Object) []byte {
	val := fmt.Sprintf("%v", obj.Value)

	// *3\r\n$3\r\nSET\r\n$<klen>\r\n<key>\r\n$<vlen>\r\n<val>\r\n
	cmd := fmt.Sprintf(
		"*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(key), key,
		len(val), val,
	)

	// Append PEXPIREAT if the key has a future absolute expiry timestamp.
	if obj.ExpiresAt != -1 && obj.ExpiresAt > time.Now().UnixMilli() {
		ts := fmt.Sprintf("%d", obj.ExpiresAt)
		cmd += fmt.Sprintf(
			"*3\r\n$9\r\nPEXPIREAT\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
			len(key), key,
			len(ts), ts,
		)
	}

	return []byte(cmd)
}

// DumpAllAOF rewrites the entire AOF file from the current in-memory store.
// It writes to a temporary file first and then atomically renames it to
// AOFFile, so a crash mid-write never leaves a corrupted AOF on disk.
func DumpAllAOF() error {
	// Open the temp file (truncate if it already exists from a previous crash).
	tmpF, err := os.OpenFile(AOFFileTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("aof: open temp file: %w", err)
	}

	w := bufio.NewWriter(tmpF)
	nowMs := time.Now().UnixMilli()

	for key, obj := range store {
		// Skip keys that are already expired.
		if obj.ExpiresAt != -1 && obj.ExpiresAt <= nowMs {
			continue
		}
		if _, err := w.Write(encodeSetCmd(key, obj)); err != nil {
			tmpF.Close()
			return fmt.Errorf("aof: write key %q: %w", key, err)
		}
	}

	if err := w.Flush(); err != nil {
		tmpF.Close()
		return fmt.Errorf("aof: flush: %w", err)
	}
	if err := tmpF.Sync(); err != nil {
		tmpF.Close()
		return fmt.Errorf("aof: fsync: %w", err)
	}
	if err := tmpF.Close(); err != nil {
		return fmt.Errorf("aof: close temp file: %w", err)
	}

	// Atomic rename: replaces AOFFile in one syscall.
	if err := os.Rename(AOFFileTmp, AOFFile); err != nil {
		return fmt.Errorf("aof: rename: %w", err)
	}

	return nil
}

// LoadAOF reads the AOF file line-by-line and replays every command into the
// in-memory store. It silently skips commands it cannot parse so that a
// partially-written tail (e.g. from a crash) does not prevent startup.
// Returns the number of commands replayed and any fatal I/O error.
func LoadAOF() (int, error) {
	f, err := os.Open(AOFFile)
	if os.IsNotExist(err) {
		// No AOF yet — this is fine on first run.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("aof: open: %w", err)
	}
	defer f.Close()

	// Read the entire file into memory for DecodeMulti.
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("aof: stat: %w", err)
	}
	buf := make([]byte, info.Size())
	if _, err := f.Read(buf); err != nil {
		return 0, fmt.Errorf("aof: read: %w", err)
	}

	values, err := DecodeMulti(buf)
	if err != nil {
		// Non-fatal: the file may have a partial last entry.
		fmt.Fprintf(os.Stderr, "aof: decode warning: %v\n", err)
	}

	replayed := 0
	for _, v := range values {
		arr, ok := v.([]interface{})
		if !ok || len(arr) == 0 {
			continue
		}
		tokens := make([]string, len(arr))
		for i, elem := range arr {
			s, ok := elem.(string)
			if !ok {
				tokens = nil
				break
			}
			tokens[i] = s
		}
		if tokens == nil {
			continue
		}
		// Replay the command; ignore errors (e.g. wrong-arity from a
		// corrupted tail — we don't want a bad last byte to abort startup).
		if _, err := Eval(tokens); err == nil {
			replayed++
		}
	}

	return replayed, nil
}
