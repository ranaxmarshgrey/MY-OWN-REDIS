package core

import (
	"fmt"
	"strconv"
)

func Decode(data []byte) (interface{}, error) {
	val, consumed, err := decodeOne(data)
	if err != nil {
		return nil, err
	}
	if consumed != len(data) {
		return nil, fmt.Errorf("trailing data after RESP value")
	}
	return val, nil
}

func decodeOne(data []byte) (interface{}, int, error) {
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("No data")

	}
	firstele := data[0]
	switch firstele {
	case '+':
		//simple string
		return decodeSimpleString(data)

	case '-':
		//error
		return decodeSimpleString(data)
	case ':':
		//integer
		return decodeInteger(data)
	case '$':
		//bulk string
		return decodeBulkString(data)
	case '*':
		//array
		return decodeArray(data)

	default:
		return nil, 0, fmt.Errorf("invalid RESP type %q", firstele)

	}
}

func decodeSimpleString(data []byte) (string, int, error) {
	line, consumed, err := readLine(data, "simple string")
	return line, consumed, err
}

func decodeInteger(data []byte) (int64, int, error) {
	line, consumed, err := readLine(data, "integer")
	if err != nil {
		return 0, 0, err
	}
	value, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid integer %q: %w", line, err)
	}
	return value, consumed, nil
}

func decodeBulkString(data []byte) (interface{}, int, error) {
	length, delta, err := readLength(data)
	if err != nil {
		return nil, 0, err
	}
	if length == -1 {
		return nil, delta, nil
	}
	if length < 0 {
		return nil, 0, fmt.Errorf("invalid bulk string length %d", length)
	}
	if delta > len(data) || length > len(data)-delta || len(data)-(delta+length) < 2 {
		return nil, 0, fmt.Errorf("bulk string payload is truncated")
	}

	payloadEnd := delta + length
	if data[payloadEnd] != '\r' || data[payloadEnd+1] != '\n' {
		return nil, 0, fmt.Errorf("bulk string is missing CRLF terminator")
	}
	return string(data[delta:payloadEnd]), payloadEnd + 2, nil

}

func readLength(data []byte) (int, int, error) {
	line, consumed, err := readLine(data, "length")
	if err != nil {
		return 0, 0, err
	}
	value, err := strconv.ParseInt(line, 10, 64)
	if err != nil || int64(int(value)) != value {
		return 0, 0, fmt.Errorf("invalid length %q", line)
	}
	return int(value), consumed, nil
}

func readLine(data []byte, valueType string) (string, int, error) {
	for pos := 1; pos < len(data); pos++ {
		if data[pos] == '\n' {
			return "", 0, fmt.Errorf("invalid %s terminator", valueType)
		}
		if data[pos] != '\r' {
			continue
		}
		if pos+1 >= len(data) || data[pos+1] != '\n' {
			return "", 0, fmt.Errorf("invalid %s terminator", valueType)
		}
		return string(data[1:pos]), pos + 2, nil
	}
	return "", 0, fmt.Errorf("truncated %s", valueType)

}

func decodeArray(data []byte) (interface{}, int, error) {
	count, delta, err := readLength(data)
	if err != nil {
		return nil, 0, err
	}
	if count == -1 {
		return nil, delta, nil
	}
	if count < 0 {
		return nil, 0, fmt.Errorf("invalid array length %d", count)
	}
	if count > len(data)-delta {
		return nil, 0, fmt.Errorf("array elements are truncated")
	}
	pos := delta
	arr := make([]interface{}, count)

	for i := 0; i < count; i++ {
		ele, d, err := decodeOne(data[pos:])
		if err != nil {
			return nil, 0, fmt.Errorf("invalid array element %d: %w", i, err)
		}
		arr[i] = ele
		pos = pos + d

	}

	return arr, pos, nil
}

func DecodeToArrayOfStrings(data []byte) ([]string, error) {
	result, err := Decode(data)
	if err != nil {
		return nil, err
	}
	arr := result.([]interface{})
	tokens := make([]string, len(arr))
	for i, v := range arr {
		tokens[i] = v.(string)
	}

	return tokens, nil
}
