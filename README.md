# Audiologger ZuidWest FM

This repository contains a bash script designed to record audio streams hourly and log relevant metadata about the current broadcast. It also ensures the periodic cleanup of old recordings.

## Features

- **Continuous Recording**: Automatically captures the audio stream from ZuidWest FM every hour.
- **Metadata Logging**: Fetches and logs the current program name from the broadcast data API, adding context to each recording.
- **Detailed Log File**: Maintains a comprehensive log file for tracking the script’s activities and any potential errors.
- **Automatic Cleanup**: Deletes audio files older than 31 days to conserve storage space.
- **Debug Mode**: Provides additional output for troubleshooting when enabled.

## Prerequisites

The script requires the following tools:
- `jq` - A command-line JSON processor.
- `curl` - A command-line tool for transferring data with URLs.
- `ffmpeg` - A command-line tool for recording, converting, and streaming audio and video.

This script is intended for use with websites based on the [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp), which utilizes the Broadcast Data API from the theme. If you are using a different API, set `PARSE_METADATA` to 0 and use a plaintext file for metadata, or implement your own parsing method.

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

Edit the script to specify the recording directory (`RECDIR`), log file path (`LOGFILE`), and other parameters:
- `STREAMURL`: The URL of the audio stream.
- `RECDIR`: The directory where audio recordings are stored.
- `LOGFILE`: The path to the log file for logging script operations.
- `METADATA_URL`: The API endpoint for fetching broadcast metadata.
- `KEEP`: The number of days to retain audio recordings.
- `PARSE_METADATA`: Enables or disables metadata parsing from the Streekomroep WordPress theme.

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

## Debugging

To enable debug mode, set `DEBUG=1` in the script. This will output debug information to the console to help identify any issues during execution.

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
