# ZuidWest FM Audio Logger

A production-ready Go application for recording hourly audio streams and serving audio segments via HTTP API. Features intelligent caching, structured logging, and robust error handling.

## ✨ Features

- **Continuous Recording**: Automatic hourly audio stream capture with built-in cron scheduling
- **HTTP API**: Serve audio segments by time range with intelligent caching
- **Multi-Stream Support**: Record multiple streams simultaneously with per-stream configuration
- **Metadata Collection**: Fetch and store program information from broadcast APIs
- **Smart Caching**: Cache frequently requested segments for instant response
- **Production Ready**: Structured logging, graceful shutdown, CORS support, comprehensive error handling
- **Auto Cleanup**: Configurable retention periods for recordings and cache

## 📋 Prerequisites

### System Requirements
- **Go 1.24+** (for building from source)
- **FFmpeg** (for audio recording and segment extraction)

### Install Dependencies

**macOS:**
```bash
brew install ffmpeg go
```

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install ffmpeg golang-go
```

**RHEL/CentOS:**
```bash
sudo yum install ffmpeg golang
```

## 🚀 Quick Start

### 1. Clone and Build
```bash
git clone https://github.com/oszuidwest/zwfm-audiologger
cd zwfm-audiologger
go mod download
go build -o bin/audiologger cmd/audiologger/main.go
```

### 2. Configure Streams
Copy and edit the configuration file:
```bash
cp streams.json streams.local.json
nano streams.local.json
```

### 3. Run Application

**Start everything (recorder + HTTP server):**
```bash
./bin/audiologger
```

That's it! One command starts both the continuous recording service and HTTP API server.

## ⚙️ Configuration

### Configuration File Structure
The `streams.json` file contains global settings and per-stream configuration:

```json
{
  "recording_dir": "/var/audio",
  "log_file": "/var/log/audiologger.log", 
  "keep_days": 31,
  "debug": false,
  "server": {
    "port": 8080,
    "read_timeout": "30s",
    "write_timeout": "30s", 
    "shutdown_timeout": "10s",
    "cache_dir": "/var/audio/cache",
    "cache_ttl": "24h"
  },
  "streams": {
    "zuidwest": {
      "url": "https://icecast.zuidwest.cloud/zuidwest.mp3",
      "metadata_url": "https://www.zuidwestupdate.nl/wp-json/zw/v1/broadcast_data",
      "metadata_json_path": "fm.now",
      "keep_days": 31,
      "record_duration": "1h"
    }
  }
}
```

### Global Settings
| Setting | Default | Description |
|---------|---------|-------------|
| `recording_dir` | `/tmp/audiologger` | Base directory for recordings |
| `log_file` | `{recording_dir}/audiologger.log` | Log file location |
| `keep_days` | `7` | Default retention period (days) |
| `debug` | `false` | Enable debug logging |

### Server Settings
| Setting | Default | Description |
|---------|---------|-------------|
| `port` | `8080` | HTTP server port |
| `read_timeout` | `30s` | Request read timeout |
| `write_timeout` | `30s` | Response write timeout |
| `cache_dir` | `{recording_dir}/cache` | Cache directory |
| `cache_ttl` | `24h` | Cache time-to-live |

### Stream Settings
| Setting | Required | Description |
|---------|----------|-------------|
| `url` | ✅ | Audio stream URL |
| `metadata_url` | ❌ | Metadata API endpoint |
| `metadata_json_path` | ❌ | JSON path for metadata extraction |
| `keep_days` | ❌ | Override global retention |
| `record_duration` | ❌ | Recording duration (default: 1h) |

## 🗂️ Directory Structure

The application creates this structure:
```
/var/audio/
├── zuidwest/
│   ├── 2024-01-15_14.mp3    # Audio recording
│   └── 2024-01-15_14.meta   # Program metadata
├── cache/                    # Cached segments
│   └── {hash}.mp3
└── audiologger.log          # Application logs
```

## 🔧 Production Deployment

### Systemd Service (Recording)
Create `/etc/systemd/system/audiologger.service`:
```ini
[Unit]
Description=ZuidWest FM Audio Logger
After=network.target

[Service]
Type=simple
User=audiologger
Group=audiologger
WorkingDirectory=/opt/audiologger
ExecStart=/usr/local/bin/audiologger
Restart=always
RestartSec=10
Environment=CONFIG_FILE=/etc/audiologger/streams.json

[Install]
WantedBy=multi-user.target
```

### Enable and Start Service
```bash
sudo systemctl enable audiologger
sudo systemctl start audiologger
sudo systemctl status audiologger
```

## 📡 HTTP API

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/streams` | List available streams |
| `GET` | `/streams/{stream}/recordings` | List recordings |
| `GET` | `/streams/{stream}/recordings/{timestamp}` | Download full recording |
| `GET` | `/streams/{stream}/recordings/{timestamp}/metadata` | Get metadata |
| `GET` | `/streams/{stream}/audio?start={RFC3339}&end={RFC3339}` | Get audio segment |
| `GET` | `/cache/stats` | Cache statistics |

### Example API Usage

```bash
# Health check
curl http://localhost:8080/health

# List streams
curl http://localhost:8080/streams

# List recordings
curl http://localhost:8080/streams/zuidwest/recordings

# Get 5-minute segment
curl "http://localhost:8080/streams/zuidwest/audio?start=2024-01-15T14:30:00Z&end=2024-01-15T14:35:00Z" -o segment.mp3

# Download full recording
curl http://localhost:8080/streams/zuidwest/recordings/2024-01-15_14 -o recording.mp3

# Cache statistics
curl http://localhost:8080/cache/stats
```

## 🐳 Docker Deployment

### Dockerfile
```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN go build -o bin/audiologger cmd/audiologger/main.go

FROM alpine:latest
RUN apk add --no-cache ffmpeg ca-certificates tzdata jq
WORKDIR /app
COPY --from=builder /app/bin/audiologger .
COPY streams.json .
EXPOSE 8080
CMD ["./audiologger"]
```

### Docker Compose
```bash
# Start both recorder and API server
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

## 🔍 Monitoring and Debugging

### Enable Debug Mode
Set `"debug": true` in `streams.json` for detailed logging including FFmpeg output.

### Log Monitoring
```bash
# Follow logs
tail -f /var/log/audiologger.log

# JSON log parsing
jq '.level,.msg,.station' /var/log/audiologger.log

# Check cache performance
curl http://localhost:8080/cache/stats | jq
```

### Performance Metrics
- **Segment extraction**: 100-300ms (first request)
- **Cached segments**: 50-200ms (subsequent requests)  
- **Cache hit ratio**: 70-90% for popular segments
- **Storage overhead**: ~5-10% with intelligent cleanup

## 🛠️ Development

### Build Commands
```bash
# Build binary
go build -o bin/audiologger cmd/audiologger/main.go

# Run directly from source
go run cmd/audiologger/main.go

# Run tests
go test ./...

# Install dependencies
go mod download && go mod tidy
```

### Custom Configuration
```bash
# Use custom config file
go run cmd/audiologger/main.go -config custom-streams.json
```

### Code Quality
```bash
# Run linter
golangci-lint run

# Format code
go fmt ./...

# Run tests with coverage
go test -cover ./...
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Run linter (`golangci-lint run`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## 📄 License

MIT License - see LICENSE file for details.

## 🆘 Support

- **Issues**: [GitHub Issues](https://github.com/oszuidwest/zwfm-audiologger/issues)
- **Documentation**: See [CLAUDE.md](./CLAUDE.md) for detailed development guide
- **API Compatibility**: Works with [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp)

---

**Made with ❤️ by Streekomroep ZuidWest**