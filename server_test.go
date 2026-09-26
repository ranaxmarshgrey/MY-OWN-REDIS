package main

import (
	"bytes"
	"my-own-redis/core"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestProcessClientBuffer(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. Incomplete command - should not execute and keep buffer intact
	client.readBuf = append(client.readBuf, []byte("*1\r\n$4\r\nPIN")...)
	processClientBuffer(client, 0)
	if len(client.readBuf) != len("*1\r\n$4\r\nPIN") {
		t.Fatalf("expected buffer to remain intact, got: %q", string(client.readBuf))
	}

	// 2. Complete the command by appending remaining bytes
	client.readBuf = append(client.readBuf, []byte("G\r\n")...)
	processClientBuffer(client, 0)
	if len(client.readBuf) != 0 {
		t.Fatalf("expected buffer to be fully drained, remaining: %q", string(client.readBuf))
	}
	reply := readReply()
	if !bytes.Equal(reply, []byte("+PONG\r\n")) {
		t.Fatalf("expected +PONG\\r\\n, got: %q", string(reply))
	}

	// 3. Pipelined commands in a single buffer
	client.readBuf = []byte("*1\r\n$4\r\nPING\r\n*2\r\n$4\r\nPING\r\n$5\r\nhello\r\n")
	processClientBuffer(client, 0)
	if len(client.readBuf) != 0 {
		t.Fatalf("expected buffer to be drained, remaining: %q", string(client.readBuf))
	}
	reply = readReply()
	expected := []byte("+PONG\r\n$5\r\nhello\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}
}

func TestProcessClientBufferSET(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. Basic SET command via RESP
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nredis\r\n")
	processClientBuffer(client, 0)
	if len(client.readBuf) != 0 {
		t.Fatalf("expected buffer to be drained, remaining: %q", string(client.readBuf))
	}
	reply := readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got: %q", string(reply))
	}

	// 2. SET with EX expiration
	client.readBuf = []byte("*5\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$2\r\nEX\r\n$2\r\n10\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got: %q", string(reply))
	}

	// 3. Pipelined SET followed by PING
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$1\r\na\r\n$1\r\nb\r\n*1\r\n$4\r\nPING\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expected := []byte("+OK\r\n+PONG\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}

	// 4. SET wrong number of arguments error
	client.readBuf = []byte("*2\r\n$3\r\nSET\r\n$3\r\nfoo\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr := []byte("-ERR wrong number of arguments\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}

	// 5. SET syntax error
	client.readBuf = []byte("*4\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n$7\r\nINVALID\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedSyntaxErr := []byte("-ERR syntax error\r\n")
	if !bytes.Equal(reply, expectedSyntaxErr) {
		t.Fatalf("expected %q, got: %q", string(expectedSyntaxErr), string(reply))
	}
}

func TestProcessClientBufferGET(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. GET non-existent key -> $-1\r\n
	client.readBuf = []byte("*2\r\n$3\r\nGET\r\n$7\r\nmissing\r\n")
	processClientBuffer(client, 0)
	reply := readReply()
	if !bytes.Equal(reply, []byte("$-1\r\n")) {
		t.Fatalf("expected $-1\\r\\n, got: %q", string(reply))
	}

	// 2. SET then GET
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$4\r\ncity\r\n$6\r\nLondon\r\n*2\r\n$3\r\nGET\r\n$4\r\ncity\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expected := []byte("+OK\r\n$6\r\nLondon\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}

	// 3. GET wrong number of arguments error
	client.readBuf = []byte("*1\r\n$3\r\nGET\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr := []byte("-ERR wrong number of arguments for 'get' command\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}
}

func TestProcessClientBufferTTL(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. TTL on non-existent key -> :-2\r\n
	client.readBuf = []byte("*2\r\n$3\r\nTTL\r\n$7\r\nmissing\r\n")
	processClientBuffer(client, 0)
	reply := readReply()
	if !bytes.Equal(reply, []byte(":-2\r\n")) {
		t.Fatalf("expected :-2\\r\\n, got: %q", string(reply))
	}

	// 2. SET then TTL for key without expiration -> :-1\r\n
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$4\r\nuser\r\n$4\r\njohn\r\n*2\r\n$3\r\nTTL\r\n$4\r\nuser\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expected := []byte("+OK\r\n:-1\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}

	// 3. SET with EX 100 then TTL -> :100\r\n or :99\r\n
	client.readBuf = []byte("*5\r\n$3\r\nSET\r\n$3\r\nsid\r\n$3\r\n123\r\n$2\r\nEX\r\n$3\r\n100\r\n*2\r\n$3\r\nTTL\r\n$3\r\nsid\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n:100\r\n")) && !bytes.Equal(reply, []byte("+OK\r\n:99\r\n")) {
		t.Fatalf("expected +OK\\r\\n:100\\r\\n or +OK\\r\\n:99\\r\\n, got: %q", string(reply))
	}

	// 4. TTL wrong number of arguments error
	client.readBuf = []byte("*1\r\n$3\r\nTTL\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr := []byte("-ERR wrong number of arguments for 'ttl' command\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}
}

func TestParseIPv4(t *testing.T) {
	tests := []struct {
		host    string
		want    [4]byte
		wantErr bool
	}{
		{"127.0.0.1", [4]byte{127, 0, 0, 1}, false},
		{"0.0.0.0", [4]byte{0, 0, 0, 0}, false},
		{"192.168.1.100", [4]byte{192, 168, 1, 100}, false},
		{"localhost", [4]byte{127, 0, 0, 1}, false},
		{"invalid-host-name-12345", [4]byte{}, true},
	}

	for _, tc := range tests {
		got, err := parseIPv4(tc.host)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseIPv4(%q) err = %v, wantErr %v", tc.host, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseIPv4(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestProcessClientBufferDEL(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. DEL non-existent key -> :0\r\n
	client.readBuf = []byte("*2\r\n$3\r\nDEL\r\n$7\r\nmissing\r\n")
	processClientBuffer(client, 0)
	reply := readReply()
	if !bytes.Equal(reply, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n, got: %q", string(reply))
	}

	// 2. SET then DEL single key -> :1\r\n
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n*2\r\n$3\r\nDEL\r\n$3\r\nfoo\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expected := []byte("+OK\r\n:1\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}

	// 3. Multi-key DEL
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$2\r\nk1\r\n$2\r\nv1\r\n*3\r\n$3\r\nSET\r\n$2\r\nk2\r\n$2\r\nv2\r\n*4\r\n$3\r\nDEL\r\n$2\r\nk1\r\n$2\r\nk2\r\n$7\r\nmissing\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expected = []byte("+OK\r\n+OK\r\n:2\r\n")
	if !bytes.Equal(reply, expected) {
		t.Fatalf("expected %q, got: %q", string(expected), string(reply))
	}

	// 4. DEL with no arguments error
	client.readBuf = []byte("*1\r\n$3\r\nDEL\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr := []byte("-ERR wrong number of arguments for 'del' command\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}
}

func TestProcessClientBufferEXPIRE(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. EXPIRE on non-existent key -> :0\r\n
	client.readBuf = []byte("*3\r\n$6\r\nEXPIRE\r\n$7\r\nmissing\r\n$2\r\n10\r\n")
	processClientBuffer(client, 0)
	reply := readReply()
	if !bytes.Equal(reply, []byte(":0\r\n")) {
		t.Fatalf("expected :0\\r\\n, got: %q", string(reply))
	}

	// 2. SET then EXPIRE then TTL
	client.readBuf = []byte("*3\r\n$3\r\nSET\r\n$4\r\nuser\r\n$4\r\njohn\r\n*3\r\n$6\r\nEXPIRE\r\n$4\r\nuser\r\n$3\r\n100\r\n*2\r\n$3\r\nTTL\r\n$4\r\nuser\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n:1\r\n:100\r\n")) && !bytes.Equal(reply, []byte("+OK\r\n:1\r\n:99\r\n")) {
		t.Fatalf("expected +OK\\r\\n:1\\r\\n:100\\r\\n or :99\\r\\n, got: %q", string(reply))
	}

	// 3. EXPIRE wrong number of arguments
	client.readBuf = []byte("*2\r\n$6\r\nEXPIRE\r\n$4\r\nuser\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr := []byte("-ERR wrong number of arguments for 'expire' command\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}

	// 4. EXPIRE invalid expire time (string instead of int)
	client.readBuf = []byte("*3\r\n$6\r\nEXPIRE\r\n$4\r\nuser\r\n$3\r\nbad\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	expectedErr = []byte("-ERR invalid expire time\r\n")
	if !bytes.Equal(reply, expectedErr) {
		t.Fatalf("expected %q, got: %q", string(expectedErr), string(reply))
	}
}

func TestProcessClientBufferPassiveAndActiveCleanup(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	client := &Client{
		fd:      fds[0],
		readBuf: make([]byte, 0),
	}

	readReply := func() []byte {
		buf := make([]byte, 1024)
		n, err := unix.Read(fds[1], buf)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return buf[:n]
	}

	// 1. SET with short PX (20ms) then GET after sleep -> passive cleanup returns $-1\r\n
	client.readBuf = []byte("*5\r\n$3\r\nSET\r\n$5\r\npass1\r\n$3\r\nval\r\n$2\r\nPX\r\n$2\r\n20\r\n")
	processClientBuffer(client, 0)
	reply := readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got: %q", string(reply))
	}

	time.Sleep(30 * time.Millisecond)

	client.readBuf = []byte("*2\r\n$3\r\nGET\r\n$5\r\npass1\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("$-1\r\n")) {
		t.Fatalf("expected $-1\\r\\n for passively expired key, got: %q", string(reply))
	}

	// 2. Active cleanup: SET with short PX (20ms), wait, run DeleteExpiredKeys(), then GET
	client.readBuf = []byte("*5\r\n$3\r\nSET\r\n$5\r\nactv1\r\n$3\r\nval\r\n$2\r\nPX\r\n$2\r\n20\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("+OK\r\n")) {
		t.Fatalf("expected +OK\\r\\n, got: %q", string(reply))
	}

	time.Sleep(30 * time.Millisecond)

	// Trigger active cleanup routine as executed periodically in the event loop
	core.DeleteExpiredKeys()

	// GET should return nil bulk string $-1\r\n
	client.readBuf = []byte("*2\r\n$3\r\nGET\r\n$5\r\nactv1\r\n")
	processClientBuffer(client, 0)
	reply = readReply()
	if !bytes.Equal(reply, []byte("$-1\r\n")) {
		t.Fatalf("expected $-1\\r\\n after active cleanup, got: %q", string(reply))
	}
}


