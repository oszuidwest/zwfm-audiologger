
# Audiologger ZuidWest FM

This repository contains a bash script designed to record audio streams on an hourly basis and log relevant metadata about the current broadcast. The script also ensures that audio old recordings are periodically cleaned up.

## Features

- **Continuous Recording**: Automatically records the audio stream from ZuidWest FM every hour.
- **Metadata Logging**: Fetches and logs the current program name from the broadcast data API, providing context for each recording.
- **Log File**: Maintains a detailed log file for tracking the script’s activities and any potential errors.
- **Automatic Cleanup**: Removes audio files older than 31 days to free up space.
- **Debug Mode**: Troubleshooting by providing additional output when enabled.

## Prerequisites

The script requires the following tools to be installed:
- `jq` - Command-line JSON processor.
- `curl` - Command line tool and library for transferring data with URLs.

This script is designed for use with websites based on the [Streekomroep WordPress Theme](https://github.com/oszuidwest/streekomroep-wp), utilizing the Broadcast Data API from the theme. If you're using a different API, metadata logging may not function correctly and will require modifications.

## Installation

1. **Clone this repository:**
   ```
   git clone https://github.com/oszuidwest/zwfm-audiologger
   cd zwfm-audiologger
   ```
2. **Ensure the script is executable:**
   ```
   chmod +x audiologger.sh
   ```

## Configuration

Modify the script to specify the recording directory (`RECDIR`), log file path (`LOGFILE`), and other parameters:
- `STREAMURL`: URL of the audio stream.
- `RECDIR`: Directory where audio recordings are stored.
- `LOGFILE`: File path for logging script operations.
- `METADATA_URL`: API endpoint for fetching broadcast metadata.
- `KEEP`: Number of days to retain audio recordings.

## Usage

Schedule the script to run every hour using cron:
1. Open your crontab:
   ```
   crontab -e
   ```
2. Add the following line to run the script at the start of every hour:
   ```
   0 * * * * /path/to/your/zwfm-audiologger/audiologger.sh
   ```

## Debugging

To enable debug mode, set `DEBUG=1` in the script. This will output debug information to the console and help identify any issues during execution.

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