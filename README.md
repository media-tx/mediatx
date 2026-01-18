<div align="center">

# MediaTX

```
╔╦╗┌─┐┌┬┐┬┌─┐╔╦╗═╗ ╦
║║║├┤  │││├─┤ ║ ╔╩╦╝
╩ ╩└─┘─┴┘┴┴ ┴ ╩ ╩ ╚═
```

**High Performance Media Server** - RTMP/SRT streaming with low latency.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/media-tx/mediatx?style=flat)](https://github.com/media-tx/mediatx/releases)
[![Stars](https://img.shields.io/github/stars/media-tx/mediatx?style=flat)](https://github.com/media-tx/mediatx/stargazers)

</div>

## ✨ Features

- 🚀 **Low Latency** - TCP_NODELAY, optimized buffers
- 📺 **RTMP Streaming** - Full RTMP protocol support
- 🎯 **GOP Cache** - Keyframe-aware forwarding
- 🔄 **Live Restream** - Multiple viewers per stream
- 📊 **Real-time Stats** - Connection & stream monitoring
- 🏗️ **Clean Architecture** - Domain-driven design

## 🚀 Quick Start

### Build & Run

```bash
cd apps/rtmp-server
make build
./rtmp-server
```

### Publish a Stream

```bash
# Using FFmpeg
ffmpeg -re -i video.mp4 -c:v libx264 -preset ultrafast -c:a aac \
  -f flv rtmp://localhost:1935/live/mystream

# Test pattern with audio
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i sine=frequency=440 \
  -c:v libx264 -preset ultrafast -c:a aac \
  -f flv rtmp://localhost:1935/live/test
```

### Play a Stream

```bash
# FFplay
ffplay rtmp://localhost:1935/live/mystream

# VLC
vlc rtmp://localhost:1935/live/mystream
```

## 📁 Architecture

```
mediatx/
├── main.go                 # Entry point
├── stream_verticle.go      # Server verticle
├── config.go               # Configuration
├── domain/                 # Domain Layer (Entities & Business Logic)
│   ├── connection.go       # RTMPConnection entity
│   ├── stream.go           # RTMPStream entity
│   ├── connection_manager.go
│   └── stream_manager.go
├── handlers/               # Protocol Layer (RTMP Protocol)
│   ├── rtmp_connection_handler.go
│   ├── rtmp_command_handler.go
│   ├── media_forwarder.go  # Media forwarding with GOP cache
│   ├── chunk_writer.go
│   └── rtmp_amf_parser.go
└── srt/                    # SRT Protocol (Optional)
```

## ⚙️ Configuration

Edit `config.json`:

```json
{
  "tcp_server": ":1935",
  "udp_server": ":1936",
  "srt_server": ":5000"
}
```

## 🔧 Performance Tuning

MediaTX is optimized for low latency:

| Setting | Value | Description |
|---------|-------|-------------|
| TCP_NODELAY | Enabled | Disable Nagle's algorithm |
| Write Buffer | 512 bytes | Small buffer for quick flush |
| GOP Cache | Enabled | Keyframe-aware forwarding |
| Flush | Immediate | Flush after each media chunk |

## 📊 Protocol Support

| Protocol | Port | Status |
|----------|------|--------|
| RTMP | 1935 | ✅ Full Support |
| SRT | 5000 | 🚧 In Progress |

## 🛠️ Development

```bash
# Run with hot reload
make dev

# Build binary
make build

# Run tests
make test
```

## 🌟 Star History

If you find MediaTX useful, please consider giving it a star! ⭐

## 🤝 Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📝 License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">

**MediaTX** - Built with ❤️ in Go

[Report Bug](https://github.com/media-tx/mediatx/issues) · [Request Feature](https://github.com/media-tx/mediatx/issues)

</div>
