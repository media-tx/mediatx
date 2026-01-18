# SRT Package

SRT (Secure Reliable Transport) package wrapper for `github.com/datarhei/gosrt`.

## Overview

This package provides a clean abstraction layer over the `gosrt` library, allowing:

- **Easy testing**: Mock Conn and Listener interfaces
- **Future flexibility**: Swap SRT library implementation without changing handlers
- **Clean API**: Simplified interface aligned with `net.Conn` pattern

## API

### Server

```go
server := srt.NewSRTServer()

// Listen mode (server)
listener, err := server.Listen("srt", ":5000", config)

// Dial mode (client)
conn, err := server.Dial("srt", "server:5000", config)
```

### Config

```go
config := srt.DefaultConfig()
config.StreamId = "live/mystream"
config.Latency = 120 * time.Millisecond
config.Passphrase = "mypassword"
config.PbKeyLen = 16
```

### Connection

```go
// Read/Write
data := make([]byte, 1316)
n, err := conn.Read(data)
_, err = conn.Write(data)

// Get StreamID
streamID := conn.StreamId()

// Set timeouts
conn.SetReadDeadline(time.Now().Add(30 * time.Second))
conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
```

### Listener

```go
for {
    conn, err := listener.Accept()
    if err != nil {
        break
    }
    
    // Handle connection
    go handleConnection(conn)
}
```

## Implementation Status

⚠️ **Current Status**: Placeholder implementation

This package provides interfaces and structure, but requires `github.com/datarhei/gosrt` dependency to be fully functional.

### To Enable Full Implementation

1. **Add dependency**:
   ```bash
   go get github.com/datarhei/gosrt
   ```

2. **Implement wrapper**:
   - See `gosrt_wrapper.go` for example implementation
   - Uncomment and adapt the code
   - Update `server.go` to use gosrt types

3. **Test integration**:
   ```bash
   go test ./srt/...
   ```

## Design

### Interfaces

The package defines clean interfaces:

- `Server`: Create listeners and dial connections
- `Listener`: Accept connections (server side)
- `Conn`: Read/Write/Close operations (client/server)

### Benefits

1. **Abstraction**: Handlers don't need to know about gosrt details
2. **Testability**: Easy to mock interfaces for unit tests
3. **Flexibility**: Can swap gosrt for another SRT library
4. **Consistency**: Similar API to `net.Conn` and `net.Listener`

## Usage in Handlers

```go
// In handlers/srt_connection_handler.go
import "github.com/fluxorio/fluxor/apps/rtmp-server/srt"

// Create SRT server
server := srt.NewSRTServer()

// Listen for connections
config := srt.DefaultConfig()
listener, err := server.Listen("srt", ":5000", config)

// Accept connection
conn, err := listener.Accept()
if err != nil {
    return err
}

// Use SRTConnection interface
handler := handlers.NewSRTConnectionHandler(...)
return handler.HandleConnection(conn)
```

## References

- [gosrt GitHub](https://github.com/datarhei/gosrt)
- [SRT Protocol Specification](https://datatracker.ietf.org/doc/html/draft-sharabayko-srt)
