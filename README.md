# ZuidWest FM Audio Logger

Automated hourly radio stream recording with commercial removal.

## Features

- Hourly recordings with format auto-detection (MP3, AAC, OGG, OPUS, FLAC)
- Commercial removal via API markers
- Multi-station support
- Metadata fetching
- Web interface for browsing/downloading
- Automatic cleanup

## Requirements

- Go 1.25.0+
- FFmpeg & ffprobe

## Installation

```bash
git clone https://github.com/oszuidwest/zwfm-audiologger.git
cd zwfm-audiologger
go build -o audiologger .
```

## Configuration

Create `config.json`:

```json
{
  "recordings_dir": "/var/audio",
  "port": 8080,
  "keep_days": 31,
  "timezone": "Europe/Amsterdam",
  "stations": {
    "station1": {
      "stream_url": "https://stream.example.com/station1.mp3",
      "api_secret": "your-secret-key",
      "metadata_url": "https://api.example.com/nowplaying",
      "metadata_path": "data.current.title",
      "parse_metadata": true
    }
  }
}
```

### Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `recordings_dir` | string | `/var/audio` | Directory for storing recordings |
| `port` | int | `8080` | HTTP server port |
| `keep_days` | int | `31` | Number of days to retain recordings |
| `timezone` | string | `UTC` | Timezone for scheduling |
| `stations` | object | required | Map of station configurations |

### Station Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stream_url` | string | Yes | URL of the audio stream |
| `api_secret` | string | No | Secret key for API authentication |
| `metadata_url` | string | No | URL to fetch metadata from |
| `metadata_path` | string | No | JSON path to extract metadata (dot notation) |
| `parse_metadata` | bool | No | Parse response as JSON (default: false) |

## Usage

```bash
./audiologger                              # Run with config.json
./audiologger -config /path/to/config.json # Custom config
./audiologger -test                        # Test mode (10s recordings)
./audiologger -version                     # Show version
```

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/health` | No | Health check |
| GET | `/status` | No | System status |
| GET | `/recordings/{path...}` | No | Browse/download recordings |
| POST | `/program/start/{station}` | Yes | Mark program start |
| POST | `/program/stop/{station}` | Yes | Mark program end |

### Authentication

```bash
# X-API-Key header (recommended)
curl -X POST -H "X-API-Key: your-secret" http://localhost:8080/program/start/station1

# Bearer token
curl -X POST -H "Authorization: Bearer your-secret" http://localhost:8080/program/start/station1
```

## How It Works

1. Records hourly at minute 0 to temporary `.mkv` files
2. Detects audio format with `ffprobe`
3. Remuxes to proper container (`.mp3`, `.aac`, `.ogg`, `.opus`, `.flac`)
4. If markers exist, trims commercials and backs up original as `.original.{ext}`
5. Daily cleanup removes old recordings

## File Structure

```
/var/audio/
├── station1/
│   ├── 2024-01-15-14.mp3              # Recording
│   ├── 2024-01-15-14.original.mp3     # Backup (if processed)
│   ├── 2024-01-15-14.recording.json   # Markers
│   └── 2024-01-15-14.meta             # Metadata
```

## Docker

```yaml
version: '3.8'
services:
  audiologger:
    build: .
    volumes:
      - ./recordings:/var/audio
      - ./config.json:/config.json:ro
    ports:
      - "8080:8080"
    restart: unless-stopped
```

## Development

```bash
go test ./...                                       # Run tests
go fmt ./...                                        # Format code
go vet ./...                                        # Static analysis
GOTOOLCHAIN=go1.25.1 deadcode ./...                # Check dead code
```

## License

MIT
