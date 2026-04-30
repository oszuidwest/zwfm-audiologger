# ZuidWest FM Audio Logger

Hourly broadcast recorder, designed for compliance archives where missing an hour is not an option. A single Go binary captures one or more radio streams, validates each finished recording, alerts on anything suspicious, and serves the archive over HTTP.

This is what runs at Streekomroep ZuidWest. Every reliability feature in the codebase exists because some real failure mode cost us a recording at some point.

## Reliability features

- **Catchup on startup.** If the process starts mid-hour with at least 60 seconds remaining in the slot, it begins recording immediately rather than waiting for the next hour. A restart never costs you a partial hour.
- **Disk-space guard.** Refuses to start a new recording when free space drops below 1 GB, instead of silently writing zero-byte files until the volume fills.
- **Post-recording validation.** Each finished file is analyzed for silence (`ffmpeg silencedetect`) and looped content (RMS autocorrelation). Files that look broken are flagged.
- **Failure alerts.** Recording failures and validation failures send email via Microsoft Graph, retried with exponential backoff (3 attempts, 1s to 30s). Recipients can be routed per station.
- **Internal scheduler.** No reliance on system cron. The Go process owns its own schedule and shuts down gracefully on SIGTERM.
- **Format detection at remux time.** `ffprobe` decides the actual container, so a station that switches codec mid-day still produces a valid file in the right wrapper.
- **Structured logging.** JSON output via `log/slog`, suitable for ingestion into any log pipeline.

## Recording flow

1. At minute 0 of every hour, each configured stream is captured to a temporary `.mkv` file for one hour.
2. `ffprobe` detects the actual codec.
3. The file is remuxed into the appropriate container (`.mp3`, `.aac`, `.ogg`, `.opus`, `.flac`).
4. If validation is enabled, the file is analyzed. Broken files are flagged and, when configured, emailed.
5. A daily cleanup job removes recordings older than `keep_days`.

## Configuration

The application looks for `config.json` in the working directory. Override with `-config /path/to/config.json`.

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

### Top level

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `recordings_dir` | string | `/var/audio` | Directory where recordings are written. |
| `port` | int | `8080` | HTTP server listen port. |
| `keep_days` | int | `31` | Days to retain recordings before cleanup. |
| `timezone` | string | `UTC` | Timezone for hour-of-day scheduling. |
| `stations` | object | required | Map of station ID to station config. |
| `validation` | object | optional | Enables post-recording validation and alerts. See below. |

### Per station

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stream_url` | string | yes | The stream to capture. |
| `metadata_url` | string | no | Optional now-playing API endpoint. |
| `metadata_path` | string | no | JSON dot-path used to extract the metadata value. No leading dot. |
| `parse_metadata` | bool | no | If true, fetch and parse JSON. If false, no metadata file is written. |

### Validation (optional)

Add a `validation` block to enable silence and loop analysis on every recording, with optional email alerts.

```json
{
  "validation": {
    "enabled": true,
    "min_duration_secs": 3500,
    "silence_threshold_db": -40.0,
    "max_silence_secs": 5.0,
    "max_loop_percent": 30.0,
    "alert": {
      "enabled": true,
      "tenant_id": "...",
      "client_id": "...",
      "client_secret": "...",
      "sender_email": "alerts@example.com",
      "default_recipients": ["ops@example.com"]
    },
    "station_recipients": {
      "station1": ["station1-team@example.com"]
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Master switch for validation. |
| `min_duration_secs` | `3500` | Recordings shorter than this are flagged. |
| `silence_threshold_db` | `-40.0` | dB level below which audio is considered silent. |
| `max_silence_secs` | `5.0` | Max continuous silence allowed before flagging. |
| `max_loop_percent` | `30.0` | Max share of audio that may resemble a loop. |
| `alert.*` | | Microsoft Graph credentials for sending email alerts. |
| `station_recipients` | | Per-station override of `default_recipients`. |

## Running

### Docker

```yaml
services:
  audiologger:
    image: ghcr.io/oszuidwest/zwfm-audiologger:latest
    container_name: audiologger
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - audiologger-data:/var/audio
      - ./config.json:/app/config.json:ro
    environment:
      - TZ=Europe/Amsterdam

volumes:
  audiologger-data:
```

The image is published as multi-arch (`linux/amd64`, `linux/arm64`) on every release. Tags follow SemVer: `5.0.0`, `5.0`, `5`, and `latest`.

### Binary

```bash
./audiologger                              # uses config.json in cwd
./audiologger -config /path/to/config.json
./audiologger -test                        # 10-second recordings, for verification
./audiologger -version
```

Pre-built binaries for `linux/amd64`, `linux/arm64`, `linux/arm/7`, `darwin/amd64`, and `darwin/arm64` are attached to every GitHub Release.

## HTTP API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness check. Returns `200 OK`. |
| GET | `/status` | Process heartbeat and current time, as JSON. |
| GET | `/recordings/{path...}` | Browse and download the recordings tree. |

## Storage layout

```
/var/audio/
├── station1/
│   ├── 2026-04-30-22.mp3      # hourly recording, container chosen by codec
│   └── 2026-04-30-22.meta     # metadata sidecar, written when metadata_url is set
└── station2/
    └── ...
```

## Development

```bash
go build -o audiologger .
go test -race -shuffle=on ./...
go fmt ./...
go vet ./...
golangci-lint run --timeout=5m
```

Requires Go 1.26.2 or higher and `ffmpeg`/`ffprobe` available in `PATH`.

## License

MIT
