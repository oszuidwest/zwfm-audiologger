# ZuidWest FM Audiologger

Automated recording system for radio streams with commercial removal.

## Features

- **Hourly recordings** - Automatic recording at the top of each hour
- **Format auto-detection** - Supports MP3, AAC, OGG, OPUS, FLAC
- **Commercial removal** - Mark program start/stop to trim commercials
- **Multi-station** - Record multiple streams simultaneously
- **Metadata support** - Fetches and stores program metadata from broadcast APIs
- **Web interface** - Browse and download recordings
- **API control** - HTTP endpoints for automation
- **Automatic cleanup** - Configurable retention period for recordings

## Installation

Requires: Go 1.23+, FFmpeg

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
  "port": 8090,
  "keep_days": 31,
  "stations": {
      "parse_metadata": true,
    "station1": {
      "stream_url": "https://stream.example.com/station1",
      "api_secret": "station1-secret",
      "metadata_url": "https://api.example.com/metadata",
      "metadata_path": "now_playing",
      "parse_metadata": true
    },
    "station2": {
      "stream_url": "https://stream.example.com/station2.aac",
      "api_secret": "station2-secret"
    }
  }
}
```

## Usage

```bash
# Run with default config
./audiologger

# Custom config file
./audiologger -config myconfig.json

# Test mode (10-second recordings)
./audiologger -test
```

## API Endpoints

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/program/start/{station}` | Mark program start | Yes |
| POST | `/program/stop/{station}` | Mark program end | Yes |
| GET | `/recordings/` | Browse recordings | No |
| GET | `/status` | Active recordings | No |
| GET | `/health` | Health check | No |

### Authentication

Each station requires its own API secret configured in the station settings. Authentication can be provided via:

- **X-API-Key header**: `curl -H "X-API-Key: station1-secret" http://localhost:8090/program/start/station1`
- **Authorization header**: `curl -H "Authorization: Bearer station1-secret" http://localhost:8090/program/start/station1`
- **Query parameter**: `curl http://localhost:8090/program/start/station1?secret=station1-secret`

## How It Works

1. **Recording**: Every hour, records streams to temporary `.rec` files using FFmpeg
2. **Format Detection**: Uses `ffprobe` to automatically detect the actual audio format (MP3, AAC, OGG, OPUS, FLAC)
3. **Rename**: Renames `.rec` files to correct extension based on detected format
4. **Post-Processing**: If markers exist, trims commercials and replaces original (backup saved as .original)
5. **Cleanup**: Old recordings are automatically removed based on `keep_days` setting

The temporary `.rec` file approach ensures that partially recorded files (from interrupted recordings) don't get served via the API, as only files with proper extensions are accessible.

### File Structure

```
/var/audio/
├── station1/
│   ├── 2024-01-15-14.mp3           # Original recording
│   ├── 2024-01-15-14_processed.mp3 # Without commercials
│   └── 2024-01-15-14.recording.json # Marker data
└── station2/
    └── 2024-01-15-14.aac
```

## Docker

```yaml
version: '3.8'
services:
  audiologger:
    build: .
    volumes:
      - ./recordings:/var/audio
      - ./config.json:/config.json
    ports:
      - "8090:8090"
    restart: unless-stopped
```

## Systemd Service

```ini
[Unit]
Description=Audio Logger
After=network.target

[Service]
Type=simple
User=audiologger
ExecStart=/usr/local/bin/audiologger -config /etc/audiologger/config.json
Restart=always

[Install]
WantedBy=multi-user.target
```

## Development

```bash
# Run tests
go test ./...

# Check for dead code
deadcode ./...

# Format code
go fmt ./...
```

## License

MIT