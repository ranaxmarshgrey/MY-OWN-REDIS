package main

import (
	"flag"
	"fmt"
	"log"
	"main/core"
	"net"
	"sync/atomic"
)

// "sync"

// "time"
// "bytes"

func main() {
	// fmt.Println("hello,go")
	var port int
	var host string
	var client_count atomic.Int64
	// var wg sync.WaitGroup
	flag.IntVar(&port, "port", 7879, "Port to listen on")
	flag.StringVar(&host, "host", "0.0.0.0", "Host address to bind to")
	flag.Parse()
	address := fmt.Sprintf("%s:%d", host, port)
	fmt.Println(address)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		client_count.Add(1)
		log.Printf("new client connected %s, Active client %d", conn.RemoteAddr(), client_count.Load())
		go handleClient(conn, &client_count)

	}

}
func readCommand(conn net.Conn) (string, error) {
	// Create a 1024-byte buffer to temporarily hold incoming data
	buf := make([]byte, 1024)

	// Ask conn to read data into the buffer
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	// Turn only the received n bytes into a string and return it
	return string(buf[:n]), nil
}

func respond(conn net.Conn, command string) {
	_, err := conn.Write([]byte(command))
	if err != nil {
		fmt.Println("Error writing to client:", err)
	}
}

func handleClient(conn net.Conn, client_count *atomic.Int64) {
	defer conn.Close()
	for {
		command, err := readCommand(conn)
		if err != nil {

			client_count.Add(-1)
			log.Printf("Client disconnected: %s , Active clients: %d", conn.RemoteAddr(), client_count.Load())
			break
		}

		tokens, err := core.DecodeToArrayOfStrings([]byte(command))
		if err != nil {
			log.Printf("error decoding command: %v", err)
			core.RespondError(err, conn)
			continue
		}
		cmd := core.ParseCommand(tokens)
		core.EvalAndRespond(cmd, conn)
	}
}
