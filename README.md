# ZuidWest FM Audio Logger

A Go application for recording hourly audio streams and serving audio segments via HTTP API.

## Features

- Continuous recording with hourly audio stream capture using cron scheduling
- HTTP API for serving audio segments by time range with intelligent caching
- Multi-stream support with per-stream configuration and bitrate detection
- Metadata collection from broadcast APIs with JSON parsing
- Dynamic bitrate detection from icecast streams
- Structured logging with slog and station context
- Graceful shutdown and comprehensive error handling
- Configurable retention periods for recordings and cache

## Key Technical Features

### Bitrate Detection
- Automatic detection from icecast headers (`icy-br`, `ice-audio-info`)
- Fallback to MP3 frame analysis for non-compliant streams
- Used for accurate file size validation with 20% tolerance

### Time Format
- Universal YYYY-MM-DD-HH format used throughout application
- Consistent timezone handling (Europe/Amsterdam)
- API responses use YYYY-MM-DD HH:MM format for display

### Logging
- Structured logging with Go's standard slog
- Station context for easy filtering
- Outputs to both file and console
- Debug mode shows FFmpeg output and detailed operations

### Error Handling
- Retry logic with exponential backoff (3 attempts)
- FFmpeg reconnection settings for network issues
- Graceful degradation when metadata APIs are unavailable

## Prerequisites

### System Requirements
- **Go 1.24+** (for building from source)
- **FFmpeg** (for audio recording and segment extraction)
- **Network access** to icecast streams and metadata APIs

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

## Quick Start

### Clone and Build
```bash
git clone https://github.com/oszuidwest/zwfm-audiologger
cd zwfm-audiologger
go mod download
go build -o audiologger .
```

### Configure Streams
Copy and edit the configuration file:
```bash
cp streams.json streams.local.json
nano streams.local.json
```

### Run Application

**Start everything (recorder + HTTP server):**
```bash
./audiologger
```

This starts both the continuous recording service and HTTP API server. The application runs as a unified service with:
- Cron-based hourly recording at minute 0 of each hour
- HTTP API server on the configured port (default 8080)
- Automatic bitrate detection for optimal recording validation
- Structured logging with station context for easy monitoring

## Configuration

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
      "stream_url": "https://icecast.zuidwest.cloud/zuidwest.mp3",
      "metadata_url": "https://www.zuidwestupdate.nl/wp-json/zw/v1/broadcast_data",
      "metadata_path": "fm.now",
      "parse_metadata": true,
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
| `debug` | `false` | Enable debug logging with FFmpeg output |

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
| `stream_url` | ✅ | Icecast stream URL (bitrate auto-detected) |
| `metadata_url` | ❌ | Metadata API endpoint |
| `metadata_path` | ❌ | gjson path for metadata extraction |
| `parse_metadata` | ❌ | Enable JSON metadata parsing (true/false) |
| `keep_days` | ❌ | Override global retention period |
| `record_duration` | ❌ | Recording duration (default: 1h) |

## Directory Structure

The application creates this structure:
```
/var/audio/
├── zuidwest/
│   ├── 2024-01-15-14.mp3    # Audio recording
│   └── 2024-01-15-14.meta   # Program metadata
├── cache/                    # Cached segments
│   └── {hash}.mp3
└── audiologger.log          # Application logs
```

## Production Deployment

### Systemd Service
Create `/etc/systemd/system/audiologger.service`:
```ini
[Unit]
Description=ZuidWest FM Audio Logger - Unified Recording & API Service
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=audiologger
Group=audiologger
WorkingDirectory=/opt/audiologger
ExecStart=/usr/local/bin/audiologger -config /etc/audiologger/streams.json
Restart=always
RestartSec=10
TimeoutStopSec=30

# Environment
Environment=TZ=Europe/Amsterdam

# Security settings
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/audio /var/log/audiologger.log

[Install]
WantedBy=multi-user.target
```

### Enable and Start Service
```bash
sudo systemctl enable audiologger
sudo systemctl start audiologger
sudo systemctl status audiologger
```

## HTTP API

### API Endpoints

#### System Endpoints
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |

#### API v1 Endpoints
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/streams` | List streams with details |
| `GET` | `/api/v1/streams/{stream}` | Get stream details |
| `GET` | `/api/v1/streams/{stream}/recordings` | List recordings with metadata |
| `GET` | `/api/v1/streams/{stream}/recordings/{timestamp}` | Get recording info |
| `GET` | `/api/v1/streams/{stream}/recordings/{timestamp}/download` | Download recording |
| `GET` | `/api/v1/streams/{stream}/recordings/{timestamp}/metadata` | Get metadata |
| `GET` | `/api/v1/streams/{stream}/segments?start={RFC3339}&end={RFC3339}` | Get audio segment |
| `GET` | `/api/v1/system/cache` | Cache statistics |
| `GET` | `/api/v1/system/stats` | System statistics |


### Example API Usage
```bash
# Health and readiness checks
curl http://localhost:8080/health
curl http://localhost:8080/ready

# List streams with detailed information
curl http://localhost:8080/api/v1/streams | jq

# Get specific stream details
curl http://localhost:8080/api/v1/streams/zuidwest | jq

# List recordings with metadata
curl http://localhost:8080/api/v1/streams/zuidwest/recordings | jq

# Get recording information
curl http://localhost:8080/api/v1/streams/zuidwest/recordings/2024-01-15-14 | jq

# Download recording
curl http://localhost:8080/api/v1/streams/zuidwest/recordings/2024-01-15-14/download -o recording.mp3

# Get metadata
curl http://localhost:8080/api/v1/streams/zuidwest/recordings/2024-01-15-14/metadata | jq

# Get 5-minute audio segment
curl "http://localhost:8080/api/v1/streams/zuidwest/segments?start=2024-01-15T14:30:00Z&end=2024-01-15T14:35:00Z" -o segment.mp3

# System statistics
curl http://localhost:8080/api/v1/system/stats | jq
curl http://localhost:8080/api/v1/system/cache | jq
```


## Docker Deployment

### Dockerfile
```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN go build -o audiologger .

FROM alpine:latest
RUN apk add --no-cache ffmpeg ca-certificates tzdata jq
WORKDIR /app
COPY --from=builder /app/audiologger .
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

## Monitoring and Debugging

### Enable Debug Mode
Set `"debug": true` in `streams.json` for detailed logging including:
- FFmpeg command output and errors
- Bitrate detection process details
- Cache hit/miss information
- Detailed HTTP request logging

### Log Monitoring
```bash
# Follow logs in real-time
tail -f /var/log/audiologger.log

# Log format is structured text with key=value pairs
grep "ERROR" /var/log/audiologger.log
grep "station=zuidwest" /var/log/audiologger.log
grep "bitrate_kbps" /var/log/audiologger.log

# Monitor recording activities
grep "recording started\|recording completed" /var/log/audiologger.log

# Check cache and system performance
curl http://localhost:8080/api/v1/system/cache | jq
curl http://localhost:8080/api/v1/system/stats | jq
```

### Performance Metrics
- **Bitrate detection**: 1-2 seconds on startup per stream
- **Segment extraction**: 100-300ms (first request)
- **Cached segments**: 50-200ms (subsequent requests)  
- **Cache hit ratio**: 70-90% for popular segments
- **Recording validation**: Based on detected bitrate with ±20% tolerance
- **Storage overhead**: ~5-10% with intelligent cleanup

## Development

### Build Commands
```bash
# Build binary
go build -o audiologger .

# Run directly from source
go run .

# Test recording with short duration
go run . -test-record

# Run tests
go test ./...

# Install dependencies
go mod download && go mod tidy
```

### Custom Configuration
```bash
# Use custom config file
go run . -config custom-streams.json
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

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Run linter (`golangci-lint run`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## License

MIT License - see LICENSE file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/oszuidwest/zwfm-audiologger/issues)
- **API Compatibility**: Works with [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp)