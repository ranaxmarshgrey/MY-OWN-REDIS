package main

import (
	"fmt"
	"log"
	"my-own-redis/core"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

type Client struct {
	fd          int
	readBuf     []byte
	// pendingCmds accumulates all fully-decoded commands from a single read
	// event before they are evaluated and flushed as one batched response.
	pendingCmds core.RedisCmds
}

var clients = make(map[int]*Client)

func parseIPv4(host string) ([4]byte, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		resolved, err := net.ResolveIPAddr("ip4", host)
		if err != nil {
			return [4]byte{}, fmt.Errorf("invalid host %q: %w", host, err)
		}
		ip = resolved.IP
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		return [4]byte{}, fmt.Errorf("host %q is not an IPv4 address", host)
	}
	var addr [4]byte
	copy(addr[:], ipv4)
	return addr, nil
}

func StartServer(host string, port int) {
	addr, err := parseIPv4(host)
	if err != nil {
		log.Fatal(err)
	}

	serverFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(serverFd)
	sa := &unix.SockaddrInet4{
		Port: port,
		Addr: addr,
	}

	if err := unix.Bind(serverFd, sa); err != nil {
		log.Fatal(err)
	}

	if err := unix.Listen(serverFd, 128); err != nil {
		log.Fatal(err)
	}

	epollFd, err := unix.EpollCreate1(0)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(epollFd)

	var event unix.EpollEvent
	event.Events = unix.EPOLLIN
	event.Fd = int32(serverFd)

	if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, serverFd, &event); err != nil {
		log.Fatal(err)
	}
	log.Printf("Server Listening on %s:%d with epoll...", host, port)
	events := make([]unix.EpollEvent, 128)

	lastCleanup := time.Now()
	cleanupInterval := 100 * time.Millisecond
	for {
		//1:sleep until something is ready
		n, err := unix.EpollWait(epollFd, events, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Fatal(err)

		}
		if time.Since(lastCleanup) > cleanupInterval {
			core.DeleteExpiredKeys()
			lastCleanup = time.Now()
		}

		//2:iterate throguh only the active sockets
		for i := 0; i < n; i++ {
			fd := events[i].Fd
			if fd == int32(serverFd) {
				//3:accept new connection
				acceptNewConnection(serverFd, epollFd)

			} else {
				//4:read data from the client socket
				handleClientData(fd, epollFd)
			}
		}

	}
}

func acceptNewConnection(serverFd int, epollFd int) {
	// 1 accept the incoming connection from the OS backlog queue.
	// clientFD is a brand-new file descripto specifically for this client
	// sa contains the client ip address and port
	clientFd, _, err := unix.Accept(serverFd)
	if err != nil {
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			//no more incoming connections to accept
			return
		}
		log.Fatal(err)
		return
	}

	//2:make the client socket non-blocking
	if err := unix.SetNonblock(clientFd, true); err != nil {
		log.Fatal(err)
		unix.Close(clientFd)
		return
	}

	//3 :register the client socket with epoll for read
	clientEvent := unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(clientFd),
	}
	if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, clientFd, &clientEvent); err != nil {
		log.Fatal(err)
		unix.Close(clientFd)
		return
	}
	clients[clientFd] = &Client{
		fd:      clientFd,
		readBuf: make([]byte, 0, 4096),
	}
	log.Printf("new client connected %d", clientFd)
}

func handleClientData(fd int32, epollFd int) {
	client, ok := clients[int(fd)]
	if !ok {
		log.Printf("client not found for fd %d", fd)
		return
	}

	tempBuf := make([]byte, 4096)
	for {
		n, err := unix.Read(int(fd), tempBuf)
		if n > 0 {
			client.readBuf = append(client.readBuf, tempBuf[:n]...)

		}
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				// no more data to read
				break
			}
			closeClient(fd, epollFd)
			return
		}
		if n == 0 {
			//client closed the connection
			closeClient(fd, epollFd)
			return
		}
	}
	processClientBuffer(client, epollFd)
}

func processClientBuffer(client *Client, epollFd int) {
	for len(client.readBuf) > 0 {
		// Attempt to parse one command at a time using DecodeOne so that
		// incomplete partial data is safely left in the buffer.
		value, delta, err := core.DecodeOne(client.readBuf)
		if err != nil {
			// Incomplete data: wait for more bytes from the client.
			break
		}

		arr, ok := value.([]interface{})
		if !ok {
			// Discard the malformed frame and move past it.
			client.readBuf = client.readBuf[delta:]
			continue
		}

		cmdTokens := make([]string, len(arr))
		invalid := false
		for i, v := range arr {
			str, ok := v.(string)
			if !ok {
				invalid = true
				break
			}
			cmdTokens[i] = str
		}

		if invalid || len(cmdTokens) == 0 {
			client.readBuf = client.readBuf[delta:]
			continue
		}

		// Collect this fully-decoded command and advance the buffer.
		cmd := core.ParseCommand(cmdTokens)
		client.pendingCmds = append(client.pendingCmds, &cmd)
		client.readBuf = client.readBuf[delta:]
	}

	// If we decoded any commands, evaluate the whole batch and write
	// all responses to the socket in a single call — the core of pipelining.
	if len(client.pendingCmds) > 0 {
		resBytes := core.EvalAndRespond(client.pendingCmds)
		if len(resBytes) > 0 {
			_, _ = unix.Write(client.fd, resBytes)
		}
		client.pendingCmds = nil
	}
}

func closeClient(fd int32, epollFd int) {
	unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), nil)
	unix.Close(int(fd))
	delete(clients, int(fd))
	log.Printf("client disconnected %d", fd)
}
