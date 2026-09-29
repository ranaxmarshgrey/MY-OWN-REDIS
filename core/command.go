package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
