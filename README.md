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
- **Waveform Visualization**: Automatic generation of peaks data for web audio players (WaveSurfer.js compatible)

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
│   ├── 2024-01-15-14.mp3.peaks.json
│   └── 2024-01-15-14.meta
└── cache/
    └── {hash}.mp3
```

## HTTP API

> **Live Demo**: The API is available at https://audiologger.zuidwest.cloud/ with stations `zuidwest` and `rucphen`.

### API Endpoints Overview

| Endpoint | Method | Description | Example |
|----------|--------|-------------|---------|
| **System** |
| `/health` | GET | Health check | `curl http://localhost:8080/health` |
| `/ready` | GET | Readiness probe with subsystem checks | `curl http://localhost:8080/ready` |
| `/api/v1/system/stats` | GET | System statistics and storage info | `curl http://localhost:8080/api/v1/system/stats` |
| **Stations** |
| `/api/v1/stations` | GET | List all configured stations | `curl http://localhost:8080/api/v1/stations` |
| `/api/v1/stations/{station}` | GET | Get specific station details | `curl http://localhost:8080/api/v1/stations/zuidwest` |
| **Recordings** |
| `/api/v1/stations/{station}/recordings` | GET | List recordings for a station | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings` |
| `/api/v1/stations/{station}/recordings/{timestamp}` | GET | Get recording details | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15` |
| `/api/v1/stations/{station}/recordings/{timestamp}/play` | GET | Stream recording (audio/mpeg) | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/play` |
| `/api/v1/stations/{station}/recordings/{timestamp}/download` | GET | Download recording file | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/download -o recording.mp3` |
| `/api/v1/stations/{station}/recordings/{timestamp}/metadata` | GET | Get recording metadata | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/metadata` |
| `/api/v1/stations/{station}/recordings/{timestamp}/peaks` | GET | Get waveform peaks data | `curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/peaks` |
| **Audio Clips** |
| `/api/v1/stations/{station}/clips` | GET | Extract audio clip by time range | `curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00&end=2025-07-15T15:35:00" -o clip.mp3` |

### Detailed Examples

#### System Endpoints

**Health Check** - Quick status check for monitoring
```bash
curl http://localhost:8080/health
```
```json
{
  "status": "ok",
  "timestamp": "2025-07-16T00:12:44.951087639+02:00",
  "uptime": "1m12.185542272s",
  "version": "5.1.0"
}
```

**Readiness Check** - Detailed subsystem status
```bash
curl http://localhost:8080/ready | jq
```
```json
{
  "data": {
    "checks": [
      {"name": "cache", "status": "ok"},
      {"name": "storage", "status": "ok"}
    ],
    "ready": true
  },
  "meta": {
    "count": 2,
    "timestamp": "2025-07-16T00:12:49.107761699+02:00",
    "version": "5.1.0"
  },
  "success": true
}
```

**System Statistics** - Storage and recording overview
```bash
curl http://localhost:8080/api/v1/system/stats | jq
```
```json
{
  "data": {
    "station_stats": {
      "zuidwest": {
        "last_recorded": "2025-07-16T00:00:00+02:00",
        "recordings": 10,
        "size_bytes": 739178788
      }
    },
    "total_recordings": 20,
    "total_size": 1478357495,
    "uptime": "1m21.109119262s"
  },
  "meta": {
    "count": 1,
    "timestamp": "2025-07-16T00:12:53.874669915+02:00",
    "version": "5.1.0"
  },
  "success": true
}
```

#### Station Endpoints

**List All Stations** - Overview of configured stations
```bash
curl http://localhost:8080/api/v1/stations | jq
```
```json
{
  "data": {
    "stations": [{
      "has_metadata": true,
      "keep_days": 31,
      "last_recorded": "2025-07-16 00:00",
      "name": "zuidwest",
      "recordings": 10,
      "status": "active",
      "total_size": 739178788,
      "url": "https://icecast.zuidwest.cloud/zuidwest.mp3"
    }]
  },
  "meta": {
    "count": 1,
    "timestamp": "2025-07-16T00:13:00.072395708+02:00",
    "version": "5.1.0"
  },
  "success": true
}
```

#### Recording Endpoints

**List Recordings** - Get all recordings for a station
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings | jq '.data.recordings[0]'
```
```json
{
  "duration": "01:00:00.000",
  "end_time": "2025-07-15 16:00",
  "has_metadata": true,
  "size": 86400804,
  "size_human": "82.4 MB",
  "start_time": "2025-07-15 15:00",
  "timestamp": "2025-07-15-15",
  "urls": {
    "details": "/api/v1/stations/zuidwest/recordings/2025-07-15-15",
    "download": "/api/v1/stations/zuidwest/recordings/2025-07-15-15/download",
    "metadata": "/api/v1/stations/zuidwest/recordings/2025-07-15-15/metadata",
    "peaks": "/api/v1/stations/zuidwest/recordings/2025-07-15-15/peaks",
    "playback": "/api/v1/stations/zuidwest/recordings/2025-07-15-15/play"
  }
}
```

**Play Recording** - Stream audio directly
```bash
# Play with ffplay
curl -s http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/play | ffplay -

# Play with mpv
mpv http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/play

# Save to file
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/download -o "zuidwest-2025-07-15-15.mp3"
```

**Get Waveform Peaks** - For audio visualization
```bash
curl http://localhost:8080/api/v1/stations/zuidwest/recordings/2025-07-15-15/peaks | jq
```
```json
{
  "data": {
    "peaks": {
      "version": 2,
      "channels": 1,
      "sample_rate": 44100,
      "samples_per_pixel": 800,
      "bits": 8,
      "length": 4500,
      "data": [120, 125, 118, 122, ...]
    },
    "station": "zuidwest",
    "timestamp": "2025-07-15-15",
    "generated": false
  },
  "meta": {
    "count": 1,
    "timestamp": "2025-07-16T00:13:00.072395708+02:00",
    "version": "5.1.0"
  },
  "success": true
}
```

The peaks data is compatible with [WaveSurfer.js](https://wavesurfer.xyz/) and similar audio visualization libraries. Peaks are automatically generated after each recording or on-demand when first requested. See the [examples directory](examples/wavesurfer-example.html) for a complete implementation.

#### Audio Clips

**Extract Custom Time Range** - Get a specific segment
```bash
# Extract 5 minutes (15:30 to 15:35)
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00&end=2025-07-15T15:35:00" \
  -o news-segment.mp3

# Extract with positive timezone offset
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00+02:00&end=2025-07-15T15:35:00+02:00" \
  -o news-segment.mp3

# Extract with negative timezone offset
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:30:00-05:00&end=2025-07-15T15:35:00-05:00" \
  -o news-segment.mp3

# Extract with UTC timezone
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T13:30:00Z&end=2025-07-15T13:35:00Z" \
  -o news-segment.mp3

# Extract 30 seconds for a promo
curl "http://localhost:8080/api/v1/stations/zuidwest/clips?start=2025-07-15T15:00:00&end=2025-07-15T15:00:30" \
  -o promo.mp3
```

> **Note**: The API accepts timezone offsets in both URL-encoded and unencoded formats. While `+02:00` works directly in most contexts, some HTTP clients may automatically encode it as `%2B02:00` - both formats work identically.

### Response Format

All API endpoints return consistent JSON responses:

```json
{
  "data": {},        // Actual response data
  "meta": {          // Metadata about the response
    "count": 1,      // Number of items returned
    "timestamp": "", // Server timestamp
    "version": ""    // API version
  },
  "success": true    // Request success status
}
```

### Error Responses

```json
{
  "error": {
    "code": 404,
    "message": "Recording not found",
    "details": "Recording '2025-07-15-25' does not exist"
  },
  "meta": {
    "timestamp": "2025-07-16T00:15:00.000000000+02:00",
    "version": "5.1.0"
  },
  "success": false
}
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