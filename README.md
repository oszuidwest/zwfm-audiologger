# ZuidWest FM Audio Logger

An automated broadcast recording system designed for radio station compliance logging and program archival.

## Features

- **Automated hourly recording** with runtime audio format detection (MP3, AAC, OGG, OPUS, FLAC)
- **Multi-station support** with concurrent stream recording
- **Broadcast metadata extraction** from external playout APIs
- **Web-based file browser** for accessing and downloading recordings
- **Automatic retention management** with configurable cleanup schedules

## Requirements

- Go 1.25.0 or higher
- FFmpeg and ffprobe

## Installation

```bash
git clone https://github.com/oszuidwest/zwfm-audiologger.git
cd zwfm-audiologger
go build -o audiologger .
```

## Configuration

The application requires a `config.json` file in the working directory:

```json
{
  "recordings_dir": "/var/audio",
  "port": 8080,
  "keep_days": 31,
  "timezone": "Europe/Amsterdam",
  "stations": {
    "station1": {
      "stream_url": "https://stream.example.com/station1.mp3",
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
| `recordings_dir` | string | `/var/audio` | Storage directory for recordings |
| `port` | int | `8080` | HTTP server listening port |
| `keep_days` | int | `31` | Retention period in days before automatic deletion |
| `timezone` | string | `UTC` | Timezone for scheduling operations |
| `stations` | object | required | Station configuration map |

### Station Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stream_url` | string | Yes | Broadcast stream URL |
| `metadata_url` | string | No | Playout system metadata API endpoint |
| `metadata_path` | string | No | JSON path for metadata extraction (dot notation) |
| `parse_metadata` | bool | No | Enable JSON parsing of metadata response (default: false) |

## Usage

```bash
./audiologger                              # Run with default config.json
./audiologger -config /path/to/config.json # Specify custom configuration file
./audiologger -test                        # Test mode with 10-second recordings
./audiologger -version                     # Display version information
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/status` | System status information |
| GET | `/recordings/{path...}` | File browser and download interface |

## How It Works

### Recording Process

The system operates on an hourly schedule, executing the following steps:

1. **Stream capture** - Records broadcast streams to temporary `.mkv` container files at the top of each hour
2. **Format detection** - Analyzes codec using `ffprobe` to determine the actual audio format
3. **Container remuxing** - Converts to the appropriate container format (`.mp3`, `.aac`, `.ogg`, `.opus`, `.flac`)
4. **Retention management** - Performs daily cleanup to remove recordings exceeding the retention period

## File Structure

```
/var/audio/
├── station1/
│   ├── 2024-01-15-14.mp3              # Hourly recording
│   └── 2024-01-15-14.meta             # Broadcast metadata from playout system
```

## Docker Deployment

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
go test ./...     # Execute test suite
go fmt ./...      # Format source code
go vet ./...      # Run static analysis
deadcode ./...    # Detect unreachable code
```

## License

MIT
