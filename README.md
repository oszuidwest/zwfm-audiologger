# ZuidWest FM Audio Logger

An automated broadcast recording system with intelligent content trimming capabilities, designed for radio station compliance logging and program archival.

## Features

- **Automated hourly recording** with runtime audio format detection (MP3, AAC, OGG, OPUS, FLAC)
- **Content trimming** via timestamp markers for removing commercial breaks and unwanted content
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
| `recordings_dir` | string | `/var/audio` | Storage directory for recordings |
| `port` | int | `8080` | HTTP server listening port |
| `keep_days` | int | `31` | Retention period in days before automatic deletion |
| `timezone` | string | `UTC` | Timezone for scheduling operations |
| `stations` | object | required | Station configuration map |

### Station Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stream_url` | string | Yes | Broadcast stream URL |
| `api_secret` | string | No | Authentication secret for API endpoints |
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

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/health` | No | Health check endpoint |
| GET | `/status` | No | System status information |
| GET | `/recordings/{path...}` | No | File browser and download interface |
| POST | `/program/start/{station}` | Yes | Mark program start timestamp |
| POST | `/program/stop/{station}` | Yes | Mark program end timestamp |

### Authentication

Protected endpoints require station-specific authentication using either method:

```bash
# X-API-Key header (recommended)
curl -X POST -H "X-API-Key: your-secret" \
  http://localhost:8080/program/start/station1

# Bearer token
curl -X POST -H "Authorization: Bearer your-secret" \
  http://localhost:8080/program/start/station1
```

## How It Works

### Recording Process

The system operates on an hourly schedule, executing the following steps:

1. **Stream capture** - Records broadcast streams to temporary `.mkv` container files at the top of each hour
2. **Format detection** - Analyzes codec using `ffprobe` to determine the actual audio format
3. **Container remuxing** - Converts to the appropriate container format (`.mp3`, `.aac`, `.ogg`, `.opus`, `.flac`)
4. **Post-processing** - Applies content trimming if timestamp markers are present
5. **Retention management** - Performs daily cleanup to remove recordings exceeding the retention period

### Content Trimming

The system provides precise content trimming through timestamp-based markers, enabling removal of unwanted content such as commercial breaks and news segments:

**During broadcast:**

1. Issue a `POST /program/start/{station}` request when your program goes on-air (after commercial break)
2. Issue a `POST /program/stop/{station}` request when your program goes off-air (before commercial break)
3. The system persists these timestamps in `{hour}.recording.json` files

**Post-broadcast processing:**

1. The system calculates time offsets relative to the recording start (top of the hour)
2. FFmpeg performs lossless trimming based on the calculated offsets
3. The original full-hour recording is preserved as `{hour}.original.{ext}`
4. The trimmed version replaces the original file to maintain consistent URLs

**Example workflow:**

```bash
# Hourly recording begins at 14:00:00
# Commercial break ends at 14:05:30 - mark program start
curl -X POST -H "X-API-Key: secret" \
  http://localhost:8080/program/start/station1

# Program ends at 14:55:20 - mark program end
curl -X POST -H "X-API-Key: secret" \
  http://localhost:8080/program/stop/station1

# Result: 2024-01-15-14.mp3 contains program content from 14:05:30 to 14:55:20 (49m50s)
# Original full-hour recording preserved as: 2024-01-15-14.original.mp3
```

**Important notes:**

- The system supports **one program per hour** (single start/stop pair)
- Multiple start markers will overwrite the previous start time
- When only a start marker is present, the recording extends to the end of the hour
- Recordings without markers are preserved in their entirety for compliance purposes
- Markers can be set in real-time during broadcast or retroactively after transmission
- Post-processing executes automatically upon recording completion

## File Structure

```
/var/audio/
├── station1/
│   ├── 2024-01-15-14.mp3              # Trimmed program content
│   ├── 2024-01-15-14.original.mp3     # Original full-hour recording (if trimmed)
│   ├── 2024-01-15-14.recording.json   # Program timestamps
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
