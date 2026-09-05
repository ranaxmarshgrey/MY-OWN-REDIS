package core

import (
	"fmt"
	"log"
	"net"
	"strings"
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

func EvalPing(args []string) ([]byte, error) {
	if len(args) == 0 {
		return Encode("PONG", true), nil
	} else if len(args) == 1 {
		return Encode(args[0], false), nil
	} else {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'ping' command")
	}

}

func Encode(val interface{}, isSimple bool) []byte {
	str, ok := val.(string)
	if !ok {
		return nil
	}
	if isSimple {
		s := "+" + str + "\r\n"
		// simpleStr:= fmt.Sprintf("+&s\r\n",str)
		return []byte(s)
	} else {
		length := len(str)

		bulkString := fmt.Sprintf("$%d\r\n%s\r\n", length, str)
		return []byte(bulkString)
	}

}

func EvalAndRespond(cmd RedisCmd, conn net.Conn) {
	command := cmd.Cmd
	switch command {
	case "PING":
		resp, err := EvalPing(cmd.Args)
		if err != nil {
			RespondError(err, conn)
			return
		}
		conn.Write(resp)
	default:
		log.Printf("Unknown command: %s", cmd.Cmd)

	}
}

func RespondError(err error, conn net.Conn) {
	str := "-" + err.Error() + "\r\n"
	em := []byte(str)
	conn.Write(em)
}
