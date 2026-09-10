# Redis Event Loop Integration Notes

## 1. Command Decoder / RESP Parser

Current parser entry point:

- `DecodeToArrayOfStrings(data []byte) ([]string, error)`
- It takes a raw `[]byte` slice.
- It does not accept a `string` or `io.Reader`.

Behavior:

- The parser expects a single complete RESP value in the input.
- It parses one value at a time, not multiple commands in one buffer.
- It uses internal decoding logic and then converts the result to a `[]string`.

Incomplete data vs syntax error:

- There is no special sentinel like `io.ErrUnexpectedEOF` or `ErrIncomplete`.
- Partial/incomplete data is currently treated as a parse/truncation error.
- Typical failure messages include things like:
  - `truncated simple string`
  - `bulk string payload is truncated`
  - `truncated length`
  - `No data`

This means your current parser is not stream-aware; it is designed for a complete RESP object at once.

## 2. Multi-Command Buffering Strategy

Current behavior:

- If the client sends multiple commands in one read, like:

  ```text
  PING\r\nPONG\r\n
  ```

  the parser is not built to return multiple commands from one buffer.

- The parser does not currently expose:
  - a consumed byte count for incremental parsing
  - `[][]string` for multiple commands at once

Instead, it is effectively single-command oriented, which means the event loop must:

- maintain a per-client read buffer
- append incoming data into that buffer
- repeatedly parse from the front of the buffer
- remove consumed bytes after each parsed command
- keep leftover bytes for the next read

## 3. Evaluator / Writer

Current evaluation pattern:

- the command is parsed into tokens
- then evaluated against a command handler
- then a raw RESP-encoded response is produced as `[]byte`

Example behavior:

- `EvalPing(args []string) ([]byte, error)` returns raw RESP bytes
- Response is something like:

  ```text
  +PONG\r\n
  ```

Write mechanism:

- There is no custom per-client output queue currently.
- The code writes directly to the socket using the connection write path.
- In other words, the current implementation is effectively synchronous direct-write behavior, not a queued write-buffer design.

## 4. Client State Structure

Current status:

- There is no per-client `Client` struct in the project yet.
- There is no per-file-descriptor read buffer or write buffer management.
- The epoll server prototype in `server.go` accepts new clients and registers them, but does not keep client state.

So for the event loop, we should create a client wrapper like:

```go
type Client struct {
    fd       int
    readBuf  []byte
    writeBuf []byte
}
```

This is necessary because the parser is not stream-aware and the current code has no buffering logic for partial reads or multiple commands per socket.

## Summary

- Parser input: `[]byte`
- Parser output: one decoded command at a time
- No dedicated incomplete-data sentinel error exists
- No built-in multi-command handling / consumed-byte API exists
- Responses are raw `[]byte`
- No custom socket write queue exists yet
- No per-client state struct exists yet

This means the event loop implementation needs a small adapter layer that adds:

- per-client read buffering
- partial RESP parsing
- command consumption tracking
- direct socket write for replies

