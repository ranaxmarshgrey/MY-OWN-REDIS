# MY-OWN-REDIS 🚀

A lightweight, custom in-memory key-value database built from scratch in Go. It implements the official Redis Serialization Protocol (RESP), an event-driven non-blocking I/O loop using Linux `epoll`, core Redis commands, TTL expiration mechanisms (passive and active), and cache eviction.

---

## 📑 Table of Contents

- [Overview](#overview)
- [Implemented Features](#implemented-features)
- [How Does It Work? (In Simple Words)](#how-does-it-work-in-simple-words)
  - [1. Understanding the Wire Protocol (RESP)](#1-understanding-the-wire-protocol-resp)
  - [2. The Epoll Event Loop (Single-Threaded Concurrency)](#2-the-epoll-event-loop-single-threaded-concurrency)
  - [3. Command Execution Flow](#3-command-execution-flow)
  - [4. Expiration Cleanup: Passive vs Active](#4-expiration-cleanup-passive-vs-active)
  - [5. Cache Eviction (Random Eviction)](#5-cache-eviction-random-eviction)
  - [6. Command Pipelining](#6-command-pipelining)
  - [7. AOF Persistence](#7-aof-persistence)
- [Supported Commands](#supported-commands)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Running the Server](#running-the-server)
  - [Connecting with `redis-cli`](#connecting-with-redis-cli)
- [Running Tests](#running-tests)
- [Project Architecture](#project-architecture)

---

## 🌟 Overview

The goal of this project is to build an in-memory key-value store that works just like Redis under the hood:
- **No external Redis libraries**: Built from ground up using standard Go and low-level Linux syscalls.
- **Protocol-compatible**: You can connect to it directly with standard Redis clients (`redis-cli`, Python `redis-py`, etc.).
- **Single-threaded, event-driven design**: Handles thousands of concurrent connections using Linux `epoll` without lock contention or thread synchronization overhead.

---

## ⚡ Implemented Features

| Feature | Description |
| :--- | :--- |
| **RESP Protocol Parser** | Parses and serializes Simple Strings, Errors, Integers, Bulk Strings, and Arrays. |
| **Stream-Aware Decoding** | Tracks bytes consumed (`DecodeOne`) to support pipelined commands and fragmented TCP packets. |
| **Epoll Event Loop** | Non-blocking socket I/O using Linux `epoll` to multiplex concurrent client connections. |
| **Core Commands** | `PING`, `SET` (with `EX` / `PX`), `GET`, `TTL`, `DEL`, and `EXPIRE`. |
| **Passive Expiration** | On-demand key eviction when accessing expired keys (*lazy deletion*). |
| **Active Expiration** | Background probabilistic sampling to purge expired keys in batches without freezing the server. |
| **Random Eviction** | Enforces a configurable key limit (`keyLimit`), evicting arbitrary keys when full while safely preserving overwrites. |
| **Command Pipelining** | Decodes and executes multiple RESP commands from a single network read, batching all replies into one write. |
| **AOF Persistence** | Dumps every live key to `appendonly.aof` on `BGREWRITEAOF`; replays the file on startup to restore data after a crash. |

---

## 🧠 How Does It Work? (In Simple Words)

### 1. Understanding the Wire Protocol (RESP)

Redis does not talk plain JSON or HTTP. It uses **RESP (Redis Serialization Protocol)**, which is simple, text-based, and extremely fast to parse:

- **Simple String**: Starts with `+`, ends with `\r\n` (e.g., `+OK\r\n`)
- **Error**: Starts with `-`, ends with `\r\n` (e.g., `-ERR unknown command\r\n`)
- **Integer**: Starts with `:`, ends with `\r\n` (e.g., `:10\r\n`)
- **Bulk String** (binary-safe string): Starts with `$<length>\r\n<data>\r\n` (e.g., `$4\r\nping\r\n`). Null bulk strings are represented as `$-1\r\n`.
- **Array**: Starts with `*<count>\r\n` followed by its elements. For example, `SET key val` sent by `redis-cli` arrives as:
  ```text
  *3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$3\r\nval\r\n
  ```

Our parser ([`core/resp.go`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/resp.go)) reads this stream and decodes commands into standard Go string slices.

---

### 2. The Epoll Event Loop (Single-Threaded Concurrency)

Instead of spawning a new OS thread or goroutine for every single connected client (which consumes memory and causes context switching), this server uses an **event loop** powered by Linux **`epoll`** ([`server.go`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/server.go)):

```text
[ Clients ] ──TCP──> [ Linux Kernel epoll ]
                            │
              "Socket #5 has data to read!"
                            ▼
               [ Single Event Loop ]
                  ├── Read bytes from Socket #5
                  ├── Decode complete RESP command
                  ├── Execute command (Get/Set/Del)
                  └── Write RESP reply back
```

1. **Non-blocking sockets**: When reading from or writing to a client socket, the server never freezes waiting for data. If no data is available, it returns immediately (`EAGAIN`).
2. **Epoll Wait**: The Linux kernel puts the process to sleep until one or more sockets actually have incoming network data ready.
3. **Per-Client Buffers**: Network data can arrive in small fragments (e.g., half a command). Each client has a read buffer. The server appends new bytes, processes whatever full commands are ready, and keeps the remainder for the next packet.

---

### 3. Command Execution Flow

When a complete command is parsed from the buffer:
1. It is passed to [`core.Eval()`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/command.go#L24).
2. The command name is normalized (case-insensitive, e.g., `set`, `Set`, `SET` all work).
3. The arguments are validated (syntax, types, optional flags).
4. The key-value store is queried or updated.
5. The result is serialized into RESP bytes and immediately written back to the client socket.

---

### 4. Expiration Cleanup: Passive vs Active

Keys can have an expiration time attached (via `SET key value EX seconds` or `EXPIRE key seconds`). To clean them up without hurting performance, two complementary strategies are used:

#### A. Passive Expiration (Lazy Cleanup)
- When a client asks for a key (using `GET`, `TTL`, `DEL`, or `EXPIRE`), we check if `ExpiresAt <= currentTime`.
- If expired, we delete it on the spot and reply as if the key never existed.
- **Why?** It is virtually zero cost during idle time because keys are only inspected when requested.

#### B. Active Expiration (Probabilistic Sampling)
- If keys are never accessed again, passive cleanup alone would let expired keys sit in memory forever.
- To prevent memory leaks, [`core.DeleteExpiredKeys()`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/store.go#L94) runs periodically:
  1. It randomly samples up to **20 keys** that have an expiration set.
  2. It deletes all keys in that sample that have already expired.
  3. If **more than 25%** of the sampled keys were expired, it repeats immediately to clean more.
  4. If **25% or fewer** were expired, it stops to avoid hogging CPU cycles.

---

### 5. Cache Eviction (Random Eviction)

In-memory databases have finite memory. When the store reaches its capacity limit (`keyLimit`), the server must free space to make room for new keys:

```text
Store is full (e.g., 5 keys)
       │
Client sends: SET new_key value
       │
       ▼
Is new_key already present?
  ├── YES ──> Simply overwrite value (no eviction needed!)
  └── NO  ──> Call evictRandom() to delete one existing key
              Insert new_key
```

- **Configurable Limit**: Defaults to `keyLimit = 5` for demonstration and testing, adjustable via [`SetKeyLimit(limit)`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/store.go#L38).
- **The Overwrite Rule**: If updating an existing key (`SET existing_key new_val`), no new slot is needed, so eviction is never triggered.
- **Random Eviction**: Go's map iteration order is randomized by design. [`evictRandom()`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/store.go#L49) picks an arbitrary key from the map and deletes it, returning `true` if an entry was removed.

---

### 6. Command Pipelining

Normally, a client sends one command and waits for the reply before sending the next. **Pipelining** lets a client send many commands all at once without waiting, cutting round-trip time dramatically.

This server handles pipelining transparently:

```text
[ Client sends 3 commands in one TCP packet ]
        │
        ▼
[ DecodeMulti() loops through the buffer ]
  ├── Decodes command 1 → PING
  ├── Decodes command 2 → SET foo bar
  └── Decodes command 3 → GET foo
        │
        ▼
[ EvalAndRespond() runs each command ]
  └── Collects all 3 replies into one buffer
        │
        ▼
[ Single socket write back to client ]
```

- [`DecodeMulti()`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/resp.go#L25) keeps consuming RESP values from the buffer until it's empty.
- [`EvalAndRespond()`](file:///home/jayanthgowda/my-own-redis/MY-OWN-REDIS/core/command.go#L65) evaluates each command and joins all replies into a single write — so the client receives everything in one network round-trip.

---

### 7. AOF Persistence

Redis stores data in RAM — so everything is lost if the process crashes. **AOF (Append-Only File)** solves this by saving a snapshot of all current keys to disk.

#### How it works here

1. **`BGREWRITEAOF`** — You (or a scheduled job) send this command. The server launches a background goroutine that:
   - Iterates over every live key in the store.
   - Skips keys that have already expired.
   - Writes each key as a RESP-encoded `SET key value` command.
   - If the key has a future TTL, also writes a `PEXPIREAT key <timestamp_ms>` line to preserve the exact expiry time.
   - Writes everything to a **temporary file first**, then renames it to `appendonly.aof` atomically — so a crash mid-write never corrupts the file.

2. **`LoadAOF()`** — Called once at server startup. It reads `appendonly.aof` and replays every command through the normal `Eval` pipeline to rebuild the in-memory state exactly as it was before the crash.

```text
[ Server crash / restart ]
        │
        ▼
[ LoadAOF() opens appendonly.aof ]
  ├── Reads: SET hello world
  ├── Reads: SET session abc123
  └── Reads: PEXPIREAT session 1727000000000
        │
        ▼
[ Store is fully restored in memory ]
```

> **Tip**: Validate the file with the official Redis tool: `redis-check-aof appendonly.aof`

---

## 🛠 Supported Commands

| Command | Usage | Description | Example |
| :--- | :--- | :--- | :--- |
| **`PING`** | `PING [message]` | Tests server connectivity | `PING` &rarr; `+PONG` |
| **`SET`** | `SET key value [EX s] [PX ms]` | Stores key-value with optional TTL | `SET user john EX 60` &rarr; `+OK` |
| **`GET`** | `GET key` | Retrieves key's value (or nil if expired/missing) | `GET user` &rarr; `"john"` |
| **`TTL`** | `TTL key` | Returns remaining time-to-live in seconds | `TTL user` &rarr; `:58` |
| **`DEL`** | `DEL key [key ...]` | Removes one or more keys; returns deleted count | `DEL k1 k2 missing` &rarr; `:2` |
| **`EXPIRE`** | `EXPIRE key seconds` | Sets or updates a key's expiration | `EXPIRE user 120` &rarr; `:1` |
| **`PEXPIREAT`** | `PEXPIREAT key unix_ms` | Sets expiry as an absolute Unix millisecond timestamp | `PEXPIREAT session 1727000000000` &rarr; `:1` |
| **`BGREWRITEAOF`** | `BGREWRITEAOF` | Dumps all live keys to `appendonly.aof` in the background | `BGREWRITEAOF` &rarr; `+Background...` |

---

## 🚀 Getting Started

### Prerequisites
- **Go**: Version 1.22+ installed
- **OS**: Linux (required for the native `epoll` syscall implementation)

### Running the Server

Clone the repository and run:

```bash
# Start server with default port (7879) and host (0.0.0.0)
go run main.go

# Or specify custom host and port
go run main.go --host 127.0.0.1 --port 6379
```

You should see:
```text
2026/09/27 02:00:00 Starting server on 0.0.0.0:7879
```

### Connecting with `redis-cli`

In a separate terminal, connect using the official Redis CLI:

```bash
redis-cli -p 7879
```

Try running commands:

```text
127.0.0.1:7879> PING
PONG

127.0.0.1:7879> SET framework "Go"
OK

127.0.0.1:7879> GET framework
"Go"

127.0.0.1:7879> SET temp_token "abc123xyz" EX 10
OK

127.0.0.1:7879> TTL temp_token
(integer) 8

127.0.0.1:7879> DEL framework temp_token
(integer) 2
```

---

## 🧪 Running Tests

The test suite covers unit tests for parsing, command evaluation, active/passive expiration, eviction rules, and full end-to-end socket testing:

```bash
# Run all tests with race condition detector
go test -v -race ./...
```

Expected output:
```text
PASS
ok  	my-own-redis        1.107s
PASS
ok  	my-own-redis/core   1.302s
```

---

## 📁 Project Architecture

```text
MY-OWN-REDIS/
├── main.go               # Server entry point & CLI flags (--host, --port)
├── server.go             # Epoll event loop, connection handling, socket I/O
├── server_test.go        # Socket-level integration tests
├── appendonly.aof        # AOF persistence file (auto-created by BGREWRITEAOF)
├── implementation-notes.md # Notes on event loop design and stream buffers
├── ARCHITECTURE_FLOW.md  # Detailed execution flow and state diagrams
└── core/
    ├── resp.go           # RESP protocol tokenizer, parser, and encoder
    ├── resp_test.go      # RESP decoding & edge-case unit tests
    ├── command.go        # Command evaluator (all supported commands)
    ├── command_test.go   # Command-level unit tests & expiration sampling tests
    ├── store.go          # In-memory dictionary, eviction, and TTL active cleanup
    ├── eviction_test.go  # Random eviction and capacity limit unit tests
    ├── aof.go            # AOF dump (DumpAllAOF) and replay (LoadAOF) logic
    └── aof_test.go       # AOF round-trip, expired-key skipping, and BGREWRITEAOF tests
```
