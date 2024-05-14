
# Audiologger for ZuidWest FM

This repository contains a bash script designed to record hourly audio streams from ZuidWest FM and log relevant metadata about the current broadcast. The script ensures that audio recordings and their associated metadata are consistently maintained and old recordings are periodically cleaned up.

## Features

- **Continuous Recording**: Automatically records the audio stream from ZuidWest FM every hour.
- **Metadata Logging**: Fetches and logs the current program name from an external API, providing context for each recording.
- **Log File Maintenance**: Maintains a detailed log file for tracking the script’s activities and any potential errors.
- **Automatic Cleanup**: Removes audio files older than 31 days to free up space.
- **Debug Mode**: Facilitates troubleshooting by providing additional output when enabled.

## Prerequisites

The script requires the following tools to be installed:
- `jq` - Command-line JSON processor.
- `wget` - Utility for non-interactive download of files from the web.
- `curl` - Command line tool and library for transferring data with URLs.

## Installation

1. **Clone this repository:**
   ```
   git clone https://github.com/yourgithub/audiologger-zuidwest
   cd audiologger-zuidwest
   ```
2. **Ensure the script is executable:**
   ```
   chmod +x audiologger.sh
   ```

## Configuration

Modify the script to specify the recording directory (`RECDIR`), log file path (`LOGFILE`), and other operational parameters:
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
   0 * * * * /path/to/your/audiologger-zuidwest/audiologger.sh
   ```

## Debugging

To enable debug mode, set `DEBUG=1` in the script. This will output debug information to the console and help identify any issues during execution.

## Contributing

Contributions are welcome. Please fork the repository, make your changes, and submit a pull request.

## License

This project is licensed under [Insert License Here] - see the LICENSE file for details.
