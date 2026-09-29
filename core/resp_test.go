package core

import (
	"testing"
)

func TestDecodeSimpleString(t *testing.T) {
	s := "+OK\r\n"
	b1 := []byte(s)

	str, _ := Decode(b1)
	if str != "OK" {
		t.Errorf("Test case failed %s", str)
	}

}
func TestDecodeInteger(t *testing.T) {
	s := ":123\r\n"
	b1 := []byte(s)

	n, _ := Decode(b1)
	value := n.(int64)
	// fmt.Printf("%v %T\n", n, n)
	if value != 123 {
		t.Errorf("Test case failed %d", n)
	}

}
func TestDecodeError(t *testing.T) {
	input := []byte("-ERR something went wrong\r\n")

	result, _ := Decode(input)

	if result != "ERR something went wrong" {
		t.Errorf("expected error message, got %v", result)
	}
}
func TestDecodeBulkString(t *testing.T) {
	input := []byte("$5\r\nhello\r\n")

	result, _ := Decode(input)

	if result != "hello" {
		t.Errorf("expected hello, got %v", result)
	}
}

func TestDecodeArray(t *testing.T) {
	input := []byte("*2\r\n$3\r\nGET\r\n$4\r\nname\r\n")

	result, _ := Decode(input)

	arr := result.([]interface{})

	if arr[0] != "GET" {
		t.Errorf("expected GET, got %v", arr[0])
	}

	if arr[1] != "name" {
		t.Errorf("expected name, got %v", arr[1])
	}
}
func TestDecodeNestedArray(t *testing.T) {
	input := []byte("*2\r\n*2\r\n:1\r\n:2\r\n+OK\r\n")

	result, _ := Decode(input)

	arr := result.([]interface{})

	nested := arr[0].([]interface{})

	if nested[0].(int64) != 1 {
		t.Errorf("expected 1, got %v", nested[0])
	}

	if nested[1].(int64) != 2 {
		t.Errorf("expected 2, got %v", nested[1])
	}

	if arr[1] != "OK" {
		t.Errorf("expected OK, got %v", arr[1])
	}
}

func TestDecodeRejectsMalformedRESP(t *testing.T) {
	tests := []string{
		"",
		"+OK\r",
		"+OK\rX",
		":abc\r\n",
		":9223372036854775808\r\n",
		"$5\r\nhi\r\n",
		"$0\r\n",
		"$-2\r\n",
		"*2\r\n:1\r\n",
		"*-2\r\n",
		"+OK\r\nextra",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if _, err := Decode([]byte(input)); err == nil {
				t.Fatalf("expected an error for %q", input)
			}
		})
	}
}

func TestDecodeNullAndEmptyValues(t *testing.T) {
	for _, input := range []string{"$-1\r\n", "*-1\r\n"} {
		value, err := Decode([]byte(input))
		if err != nil {
			t.Fatalf("Decode(%q) returned an error: %v", input, err)
		}
		if value != nil {
			t.Errorf("Decode(%q) = %v, want nil", input, value)
		}
	}

	value, err := Decode([]byte("$0\r\n\r\n"))
	if err != nil {
		t.Fatalf("Decode empty bulk string returned an error: %v", err)
	}
	if value != "" {
		t.Errorf("Decode empty bulk string = %v, want empty string", value)
	}
}

func TestDecodeOne(t *testing.T) {
	// Test single command with consumed bytes
	input := []byte("*1\r\n$4\r\nPING\r\n")
	val, delta, err := DecodeOne(input)
	if err != nil {
		t.Fatalf("DecodeOne failed: %v", err)
	}
	if delta != len(input) {
		t.Errorf("expected delta %d, got %d", len(input), delta)
	}
	arr, ok := val.([]interface{})
	if !ok || len(arr) != 1 || arr[0] != "PING" {
		t.Errorf("unexpected value: %v", val)
	}

	// Test streaming multiple commands (pipelined data)
	stream := []byte("*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n")
	val1, delta1, err1 := DecodeOne(stream)
	if err1 != nil {
		t.Fatalf("DecodeOne command 1 failed: %v", err1)
	}
	if delta1 != 14 {
		t.Errorf("expected delta1 14, got %d", delta1)
	}
	if arr1, ok := val1.([]interface{}); !ok || arr1[0] != "PING" {
		t.Errorf("unexpected command 1: %v", val1)
	}

	// Next command starting at delta1
	val2, delta2, err2 := DecodeOne(stream[delta1:])
	if err2 != nil {
		t.Fatalf("DecodeOne command 2 failed: %v", err2)
	}
	if delta2 != 14 {
		t.Errorf("expected delta2 14, got %d", delta2)
	}
	if arr2, ok := val2.([]interface{}); !ok || arr2[0] != "PING" {
		t.Errorf("unexpected command 2: %v", val2)
	}
}

