package main

import (
	"flag"
)

func main() {
	var port int
	var host string
	flag.IntVar(&port, "port", 7879, "Port to listen on")
	flag.StringVar(&host, "host", "0.0.0.0", "Host address to bind to")
	flag.Parse()

	StartServer(host, port)
}
