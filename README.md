# ZuidWest FM Audio Logger

A Go application for recording hourly audio streams and serving audio segments via HTTP API.

## Features

- **Continuous Recording**: Hourly audio stream capture using cron scheduling
- **Format-Agnostic Recording**: Automatic detection and support for MP3, AAC, and M4A streams
- **HTTP API**: Serve audio segments by time range with caching
- **Multi-stream Support**: Per-stream configuration and automatic bitrate detection
- **Metadata Collection**: Broadcast API integration with JSON parsing
- **Dynamic Bitrate Detection**: ffprobe-based stream analysis with retry logic
- **Structured Logging**: slog-based logging with station context
- **Caching**: Segment serving with automatic cache management
- **Configurable Timezone**: Deploy anywhere with timezone-aware recording and API

## Prerequisites

### System Requirements
- **Go 1.24+** (for building from source)
- **FFmpeg** (for audio recording and segment extraction)
- **Network access** to icecast streams and metadata APIs

### Install Dependencies

```bash
# macOS
brew install ffmpeg go

# Ubuntu/Debian
sudo apt install ffmpeg golang-go

# RHEL/CentOS
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

### Configure Stations
```bash
cp streams.json streams.local.json
# Edit streams.local.json
```

### Run Application
```bash
./audiologger
```

Starts both recording service and HTTP API server.

## Configuration

### Basic Configuration Example
```json
{
  "recordings_directory": "/var/audio",
  "log_file": "/var/log/audiologger.log", 
  "keep_days": 31,
  "debug": false,
  "timezone": "Europe/Amsterdam",
  "server": {
    "port": 8080,
    "read_timeout": "30s",
    "write_timeout": "30s", 
    "shutdown_timeout": "10s",
    "cache_directory": "/var/audio/cache",
    "cache_ttl": "24h"
  },
  "stations": {
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
| `recordings_directory` | `/tmp/audiologger` | Base directory for recordings |
| `log_file` | `/var/log/audiologger.log` | Log file location |
| `keep_days` | `7` | Default retention period (days) |
| `debug` | `false` | Enable debug logging with FFmpeg output |
| `timezone` | `Europe/Amsterdam` | Timezone for recordings and API |

### Server Settings
| Setting | Default | Description |
|---------|---------|-------------|
| `port` | `8080` | HTTP server port |
| `read_timeout` | `30s` | Request read timeout |
| `write_timeout` | `30s` | Response write timeout |
| `shutdown_timeout` | `10s` | Graceful shutdown timeout |
| `cache_directory` | `{recordings_directory}/cache` | Cache directory |
| `cache_ttl` | `24h` | Cache time-to-live |

### Station Settings
| Setting | Required | Description |
|---------|----------|-------------|
| `stream_url` | ✅ | Icecast livestream URL |
| `metadata_url` | ❌ | Metadata API endpoint |
| `metadata_path` | ❌ | gjson path for metadata extraction |
| `parse_metadata` | ❌ | Enable JSON metadata parsing (true/false) |
| `keep_days` | ❌ | Override global retention period |
| `record_duration` | ❌ | Recording duration (default: 1h) |

### Timezone Configuration

Set the `timezone` field to any valid IANA timezone identifier (e.g., `Europe/Amsterdam`, `America/New_York`, `UTC`).

## Directory Structure

```
/var/audio/
├── zuidwest/
│   ├── 2024-01-15-14.mp3
│   └── 2024-01-15-14.meta
└── cache/
    └── {hash}.mp3
```

## HTTP API

### API Endpoints

#### System Endpoints

**Health Check**
```bash
curl http://localhost:8080/health
# Returns: {"status":"ok","timestamp":"2025-07-16T00:12:44.951087639+02:00","uptime":"1m12.185542272s","version":"edge"}
```

**Readiness Check**
```bash
curl http://localhost:8080/ready
# Returns: {"data":{"checks":[{"name":"cache","status":"ok"},{"name":"storage","status":"ok"}],"ready":true},"meta":{"count":2,"timestamp":"2025-07-16T00:12:49.107761699+02:00","version":"edge"},"success":true}
```

**System Statistics**
```bash
curl http://localhost:8080/api/v1/system/stats | jq
# Returns: {"data":{"station_stats":{"zuidwest":{"last_recorded":"2025-07-16T00:00:00+02:00","recordings":10,"size_bytes":739178788}},"total_recordings":20,"total_size":1478357495,"uptime":"1m21.109119262s"},"meta":{"count":1,"timestamp":"2025-07-16T00:12:53.874669915+02:00","version":"edge"},"success":true}
```

#### Station Endpoints

**List All Stations**
```bash
curl http://localhost:8080/api/v1/stations | jq
# Returns: {"data":{"stations":[{"has_metadata":true,"keep_days":31,"last_recorded":"2025-07-16 00:00","name":"zuidwest","recordings":10,"status":"active","total_size":739178788,"url":"https://icecast.zuidwest.cloud/zuidwest.mp3"}]},"meta":{"count":2,"timestamp":"2025-07-16T00:13:00.072395708+02:00","version":"edge"},"success":true}
```

**Get Station Details**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest | jq
# Returns: {"data":{"has_metadata":true,"keep_days":31,"last_recorded":"2025-07-16 00:00","name":"zuidwest","recordings":10,"status":"active","total_size":739178788,"url":"https://icecast.zuidwest.cloud/zuidwest.mp3"},"meta":{"count":1,"timestamp":"2025-07-16T00:13:05.839317331+02:00","version":"edge"},"success":true}
```

#### Recording Endpoints

**List Station Recordings**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings | jq
# Returns: {"data":{"recordings":[{"duration":"1h","end_time":"2025-07-15 16:00","has_metadata":true,"size":86400804,"size_human":"82.4 MB","start_time":"2025-07-15 15:00","timestamp":"2025-07-15-15","urls":{"details":"/api/v1/stations/zuidwest/recordings/2025-07-15-15","download":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/download","metadata":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/metadata","playback":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/play"}}]},"meta":{"count":10,"timestamp":"2025-07-16T00:13:12.468589845+02:00","version":"edge"},"success":true}
```

**Get Recording Information**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15 | jq
# Returns: {"data":{"duration":"1h","end_time":"2025-07-15 16:00","has_metadata":true,"metadata":"Moor in de Middag","size":86400804,"size_human":"82.4 MB","start_time":"2025-07-15 15:00","timestamp":"2025-07-15-15","urls":{"details":"/api/v1/stations/zuidwest/recordings/2025-07-15-15","download":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/download","metadata":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/metadata","playback":"/api/v1/stations/zuidwest/recordings/2025-07-15-15/play"}},"meta":{"count":1,"timestamp":"2025-07-16T00:13:20.731968577+02:00","version":"edge"},"success":true}
```

**Play Recording (Stream)**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/play
# Returns: Audio stream for direct playback
```

**Download Recording**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/download -o recording.mp3
# Downloads the complete recording file
```

**Get Recording Metadata**
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/metadata | jq
# Returns: {"data":{"fetched_at":"2025-07-16 00:13","metadata":"Moor in de Middag","station":"zuidwest","timestamp":"2025-07-15-15"},"meta":{"count":1,"timestamp":"2025-07-16T00:13:28.466867874+02:00","version":"edge"},"success":true}
```

#### Audio Clips

**Extract Audio Clip**
```bash
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00&end=2025-07-15T15:35:00" -o clip.mp3
# Returns: 5-minute audio segment from 15:30 to 15:35

# Also supports timezone offsets
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00+02:00&end=2025-07-15T15:35:00+02:00" -o clip.mp3
```

## Docker Deployment

### Using Pre-built Container
```bash
# Pull the latest image
docker pull ghcr.io/oszuidwest/zwfm-audiologger:latest

# Run with basic configuration
docker run -d \
  --name audiologger \
  -p 8080:8080 \
  -v $(pwd)/data:/var/audio \
  -v $(pwd)/streams.json:/app/streams.json:ro \
  -v $(pwd)/logs:/var/log \
  ghcr.io/oszuidwest/zwfm-audiologger:latest
```

### Docker Compose
```bash
docker-compose up -d
```

## Production Deployment

### Systemd Service
Create `/etc/systemd/system/audiologger.service`:
```ini
[Unit]
Description=ZuidWest FM Audio Logger
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

# Security settings
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/audio /var/log/audiologger.log

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable audiologger
sudo systemctl start audiologger
sudo systemctl status audiologger
```

## Monitoring and Debugging

### Debug Mode
Set `"debug": true` in `streams.json` for detailed logging.

### Log Monitoring
```bash
tail -f /var/log/audiologger.log
grep "ERROR" /var/log/audiologger.log
```

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
2. Create a feature branch (`git checkout -b feature/new-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Run linter (`golangci-lint run`)
6. Commit your changes (`git commit -m 'Add new feature'`)
7. Push to the branch (`git push origin feature/new-feature`)
8. Open a Pull Request

## License

MIT License - see LICENSE file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/oszuidwest/zwfm-audiologger/issues)
- **API Compatibility**: Works with [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp)