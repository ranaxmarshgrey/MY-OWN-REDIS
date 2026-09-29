package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// RedisCmds represents a batch of pipelined commands decoded from a single
// network payload. Using a named slice type keeps helper-function signatures
// clean and self-documenting.
type RedisCmds []*RedisCmd

type RedisCmd struct {
	Cmd  string
	Args []string
}

func ParseCommand(tokens []string) RedisCmd {
	var cmd RedisCmd
	cmd.Cmd = strings.ToUpper(tokens[0])
	cmd.Args = tokens[1:]

	return cmd
}

// ReadCommands decodes all concatenated RESP commands from a raw byte buffer
// and returns them as a RedisCmds batch ready for evaluation.
// Incomplete or un-parseable payloads return an error.
func ReadCommands(data []byte) (RedisCmds, error) {
	values, err := DecodeMulti(data)
	if err != nil {
		return nil, err
	}

	var cmds RedisCmds
	for _, v := range values {
		arr, ok := v.([]interface{})
		if !ok {
			return nil, fmt.Errorf("ERR expected array, got %T", v)
		}
		tokens := make([]string, len(arr))
		for i, elem := range arr {
			str, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("ERR non-string argument in command")
			}
			tokens[i] = str
		}
		if len(tokens) == 0 {
			continue
		}
		cmd := ParseCommand(tokens)
		cmds = append(cmds, &cmd)
	}
	return cmds, nil
}

// EvalAndRespond evaluates every command in the pipelined batch, collects all
// RESP-encoded response bytes into an in-memory buffer, and returns the
// complete payload to the caller for a single socket write.
func EvalAndRespond(cmds RedisCmds) []byte {
	var buf bytes.Buffer
	for _, cmd := range cmds {
		// Reconstruct the full tokens slice expected by Eval: [command, arg1, arg2, ...]
		tokens := append([]string{cmd.Cmd}, cmd.Args...)
		resBytes, err := Eval(tokens)
		if err != nil {
			msg := err.Error()
			if strings.HasPrefix(msg, "ERR") {
				resBytes = []byte(fmt.Sprintf("-%s\r\n", msg))
			} else {
				resBytes = []byte(fmt.Sprintf("-ERR %s\r\n", msg))
			}
		}
		buf.Write(resBytes)
	}
	return buf.Bytes()
}

func Eval(tokens []string) ([]byte, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("ERR empty command")
	}
	cmd := ParseCommand(tokens)
	switch cmd.Cmd {
	case "PING":
		return EvalPing(cmd.Args)

	case "SET":
		return EvalSet(cmd.Args)

	case "GET":
		return EvalGet(cmd.Args)

	case "TTL":
		return EvalTtl(cmd.Args)

	case "DEL":
		return EvalDel(cmd.Args)

	case "EXPIRE":
		return EvalExpire(cmd.Args)

	case "PEXPIREAT":
		return EvalPExpireAt(cmd.Args)

	case "BGREWRITEAOF":
		return EvalBgRewriteAOF(cmd.Args)

	default:
		return nil, fmt.Errorf("ERR unknown command '%s'", cmd.Cmd)
	}
}

func EvalPing(args []string) ([]byte, error) {
	if len(args) == 0 {
		return Encode("PONG", true), nil
	} else if len(args) == 1 {
		return Encode(args[0], false), nil
	} else {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'ping' command")
	}

}

func EvalSet(args []string) ([]byte, error) {

	if len(args) < 2 {
		return nil, errors.New("ERR wrong number of arguments")

	}
	key := args[0]
	value := args[1]
	var durationMs int64 = -1

	for i := 2; i < len(args); i++ {
		arg := strings.ToUpper(args[i])

		if arg == "EX" && i+1 < len(args) {
			sec, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil || sec <= 0 {
				return nil, errors.New("ERR invalid expire time")
			}
			durationMs = sec * 1000
			i++

		} else if arg == "PX" && i+1 < len(args) {
			ms, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil || ms <= 0 {
				return nil, errors.New("ERR invalid expire time")
			}
			durationMs = ms
			i++
		} else {
			return nil, errors.New("ERR syntax error")
		}
	}

	obj := NewObject(value, durationMs)
	Put(key, obj)
	return RESP_OK, nil

}

func EvalGet(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("ERR wrong number of arguments for 'get' command")
	}

	key := args[0]
	obj := Get(key)
	if obj == nil {
		return Encode(nil, false), nil
	}
	return Encode(obj.Value, false), nil
}

func EvalTtl(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, errors.New("ERR wrong number of arguments for 'ttl' command")
	}
	key := args[0]
	obj := Get(key)
	if obj == nil {
		return Encode(int64(-2), false), nil
	}
	if obj.ExpiresAt == -1 {
		return Encode(int64(-1), false), nil
	}

	nowMs := time.Now().UnixMilli()
	durationLeft := obj.ExpiresAt - nowMs
	if durationLeft <= 0 {
		Delete(key)
		return Encode(int64(-2), false), nil
	}

	secondsLeft := durationLeft / 1000

	return Encode(int64(secondsLeft), false), nil
}

func EvalDel(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, errors.New("ERR wrong number of arguments for 'del' command")
	}
	count := 0

	for _, key := range args {
		if Delete(key) {
			count++
		}
	}
	return Encode(int64(count), false), nil
}

func EvalExpire(args []string) ([]byte, error) {
	if len(args) != 2 {
		return nil, errors.New("ERR wrong number of arguments for 'expire' command")

	}
	key := args[0]
	seconds, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || seconds <= 0 {
		return nil, errors.New("ERR invalid expire time")
	}

	obj := Get(key)
	if obj == nil {
		return Encode(int64(0), false), nil
	}

	obj.ExpiresAt = time.Now().UnixMilli() + seconds*1000
	return Encode(int64(1), false), nil
}

// EvalPExpireAt sets the absolute expiry timestamp (Unix ms) for a key.
// This command is written to the AOF by DumpAllAOF so TTLs survive restarts.
func EvalPExpireAt(args []string) ([]byte, error) {
	if len(args) != 2 {
		return nil, errors.New("ERR wrong number of arguments for 'pexpireat' command")
	}
	key := args[0]
	ts, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || ts <= 0 {
		return nil, errors.New("ERR invalid expire time in 'pexpireat' command")
	}

	obj := Get(key)
	if obj == nil {
		return Encode(int64(0), false), nil
	}
	obj.ExpiresAt = ts
	return Encode(int64(1), false), nil
}

// EvalBgRewriteAOF forks the AOF dump into a goroutine and immediately
// returns "+Background append only file rewriting started" — matching
// Redis's own response so existing clients don't break.
func EvalBgRewriteAOF(args []string) ([]byte, error) {
	go func() {
		if err := DumpAllAOF(); err != nil {
			fmt.Fprintf(os.Stderr, "aof background rewrite error: %v\n", err)
		}
	}()
	return []byte("+Background append only file rewriting started\r\n"), nil
}

var RESP_OK = []byte("+OK\r\n")

func Encode(val interface{}, isSimple bool) []byte {

	//this older verison of encode only encode the simple string or bulk string

	// str, ok := val.(string)
	// if !ok {
	// 	return nil
	// }
	// if isSimple {
	// 	s := "+" + str + "\r\n"
	// 	// simpleStr:= fmt.Sprintf("+&s\r\n",str)
	// 	return []byte(s)
	// } else {
	// 	length := len(str)

	// 	bulkString := fmt.Sprintf("$%d\r\n%s\r\n", length, str)
	// 	return []byte(bulkString)
	// }

	//new version of encode
	switch v := val.(type) {
	case nil:
		return []byte("$-1\r\n")

	case int64:
		return []byte(fmt.Sprintf(":%d\r\n", v))

	case string:
		if isSimple {
			return []byte("+" + v + "\r\n")
		}
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(v), v))

	default:
		return nil

	}

}
