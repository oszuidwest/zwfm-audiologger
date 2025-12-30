# ZuidWest FM Audio Logger

An automated broadcast recording system designed for radio station compliance logging and program archival.

## Features

- **Automated hourly recording** with runtime audio format detection (MP3, AAC, OGG, OPUS, FLAC)
- **Multi-station support** with concurrent stream recording
- **Immediate start** - begins recording immediately when started mid-hour
- **Broadcast metadata extraction** from external playout APIs
- **Web-based file browser** for accessing and downloading recordings
- **Alerting system** with webhook and email notifications for stream failures
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
  "alerting": {
    "webhook_url": "https://hooks.example.com/alert",
    "email": {
      "enabled": true,
      "smtp_host": "mail.internal",
      "smtp_port": 587,
      "smtp_user": "",
      "smtp_pass": "",
      "smtp_starttls": true,
      "from": "audiologger@example.com",
      "to": ["ops@example.com"]
    },
    "disk_threshold_percent": 10,
    "incomplete_threshold_seconds": 3000
  },
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
| `alerting` | object | optional | Alerting configuration (see below) |
| `stations` | object | required | Station configuration map |

### Station Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stream_url` | string | Yes | Broadcast stream URL |
| `metadata_url` | string | No | Playout system metadata API endpoint |
| `metadata_path` | string | No | JSON path for metadata extraction (dot notation) |
| `parse_metadata` | bool | No | Enable JSON parsing of metadata response (default: false) |

### Alerting Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `webhook_url` | string | - | URL for webhook alerts (HTTP POST) |
| `email.enabled` | bool | `false` | Enable email alerting |
| `email.smtp_host` | string | - | SMTP server hostname |
| `email.smtp_port` | int | `587` | SMTP server port |
| `email.smtp_user` | string | - | SMTP username (optional) |
| `email.smtp_pass` | string | - | SMTP password (optional) |
| `email.smtp_starttls` | bool | `true` | Use StartTLS for SMTP connection |
| `email.from` | string | - | Sender email address |
| `email.to` | array | - | Recipient email addresses |
| `disk_threshold_percent` | int | `10` | Alert when disk space falls below this percentage |
| `incomplete_threshold_seconds` | int | `3000` | Alert when recording is shorter than this (50 min default) |

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

## Alerting

The alerting system notifies operators when recording issues occur. Both webhook and email alerting are optional and can be used independently or together.

### Alert Events

| Event | Trigger | Description |
|-------|---------|-------------|
| `stream_offline` | FFmpeg connection failure | Sent after 2-minute connection timeout |
| `stream_recovered` | Successful recording after failure | Indicates stream is back online |
| `recording_incomplete` | Duration below threshold | Recording shorter than configured minimum |
| `disk_space_low` | Disk space below threshold | Disk usage exceeds configured percentage |

### Webhook Payload

```json
{
  "type": "stream_offline",
  "station": "station-name",
  "timestamp": "2024-01-15T14:00:00Z",
  "message": "Failed to connect to stream after timeout",
  "details": {
    "error": "connection refused"
  }
}
```

### Rate Limiting

The alerting system implements rate limiting to prevent alert flooding:
- **First failure**: Alert is sent immediately
- **Repeated failures**: Suppressed while station remains offline
- **Recovery**: Alert sent when station comes back online

## How It Works

### Recording Process

The system operates on an hourly schedule with immediate start capability:

1. **Immediate start** - If started mid-hour, begins recording immediately until the end of the current hour
2. **Stream capture** - Records broadcast streams to temporary `.mkv` container files at the top of each hour
3. **Format detection** - Analyzes codec using `ffprobe` to determine the actual audio format
4. **Container remuxing** - Converts to the appropriate container format (`.mp3`, `.aac`, `.ogg`, `.opus`, `.flac`)
5. **Retention management** - Performs daily cleanup to remove recordings exceeding the retention period

### Connection Handling

- **2-minute timeout**: If FFmpeg cannot connect within 2 minutes, the recording fails and an alert is triggered
- **Reconnection support**: FFmpeg parameters enable automatic reconnection during temporary stream interruptions
- **Recovery detection**: When a previously failed stream records successfully, a recovery alert is sent

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
```

## License

MIT
