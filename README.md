# Audiologger ZuidWest FM
This repository contains a bash script designed to record audio streams hourly and log relevant metadata about the current broadcast. It also ensures the periodic cleanup of old recordings.

## Features
- **Continuous Recording**: Automatically captures audio streams every hour.
- **Metadata Logging**: Fetches and logs the current program name from broadcast data APIs, adding context to each recording.
- **Detailed Log File**: Maintains a comprehensive log file for tracking the script's activities and any potential errors.
- **Automatic Cleanup**: Deletes audio files based on configurable retention periods.
- **Debug Mode**: Provides additional output for troubleshooting when enabled.
- **Multi-Stream Support**: Can record multiple streams simultaneously with different configurations.

## Prerequisites
The script requires the following tools:
- `jq` - A command-line JSON processor.
- `curl` - A command-line tool for transferring data with URLs.
- `ffmpeg` - A command-line tool for recording, converting, and streaming audio and video.

This script is intended for use with websites based on the [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp), which utilizes the Broadcast Data API from the theme. If you are using a different API, set `parse_metadata` to 0 and use a plaintext file for metadata.

## Installation
1. **Clone this repository:**
   ```bash
   git clone https://github.com/oszuidwest/zwfm-audiologger
   cd zwfm-audiologger
   ```
2. **Ensure the script is executable:**
   ```
   chmod +x audiologger.sh
   ```

## Configuration
Configuration is done through `streams.json`. The file has two main sections: `global` and `streams`.

### Global Settings
```json
{
  "global": {
    "rec_dir": "/var/audio",      // Where to store recordings
    "log_file": "/var/log/audiologger.log",  // Log file location
    "keep_days": 31,              // Default retention period
    "debug": 1                    // Enable console logging
  }
}
```

All global settings can be customized.

### Stream Settings
Each stream in the `streams` section can have these settings:

```json
{
  "streams": {
    "stream_name": {                    // Name used for subdirectory
      "stream_url": "https://...",      // Stream URL
      "metadata_url": "https://...",    // Metadata URL
      "metadata_path": ".some.path",    // JSON path for metadata (if parsing)
      "parse_metadata": 1,              // Parse JSON (1) or use raw response (0)
      "keep_days": 31                   // Override global keep_days
    }
  }
}
```

#### Customizable per stream:
- `stream_url`: The URL of the audio stream
- `metadata_url`: Where to fetch program information
- `metadata_path`: JSON path for metadata extraction (only if parse_metadata: 1)
- `parse_metadata`: Whether to parse JSON response (1) or use raw response (0)
- `keep_days`: How long to keep recordings

#### Fixed settings (do not override):
- Recording time is fixed at 1 hour (3600 seconds)
- Network settings:
  - `reconnect_delay_max`: 300 seconds
  - `rw_timeout`: 10000000
  - Error codes: 404, 500, 503

### Directory Structure
The script creates:
```
/var/audio/
  ├── stream_name1/
  │   ├── 2024-12-19_14.mp3
  │   └── 2024-12-19_14.meta
  └── stream_name2/
      ├── 2024-12-19_14.mp3
      └── 2024-12-19_14.meta
```

## Usage
Schedule the script to run every hour using cron:
1. Open your crontab:
   ```bash
   crontab -e
   ```
2. Add the following line to run the script at the start of every hour:
   ```bash
   0 * * * * /path/to/your/zwfm-audiologger/audiologger.sh
   ```

## Docker Usage
When using Docker, mount your config file and specify directories in docker-compose:

```yaml
services:
  audiologger:
    volumes:
      - ./audio:/var/audio
      - ./logs:/var/log
      - ./streams.json:/app/streams.json:ro
```

## Debugging
To enable debug mode, set `debug: 1` in the global section of streams.json. This will output debug information to the console to help identify any issues during execution.

## Contributing
Contributions are welcome. Please fork the repository, make your changes, and submit a pull request.

# MIT License
Copyright (c) 2024 Streekomroep ZuidWest

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.