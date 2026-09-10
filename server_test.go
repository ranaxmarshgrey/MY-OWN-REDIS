package main

import (
	"bytes"
	"testing"

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

