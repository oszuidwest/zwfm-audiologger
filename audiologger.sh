#!/bin/bash

# Configuration
STREAMURL='https://icecast.zuidwestfm.nl/zuidwest.mp3'
RECDIR='/var/audio'
LOGFILE='/var/log/audiologger.log'
METADATA_URL='https://www.zuidwestupdate.nl/wp-json/zw/v1/broadcast_data'
# Output date and hour, e.g., "2023_12_31_20"
TIMESTAMP=$(/bin/date +"%Y-%m-%d_%H")
# Number of days to keep the audio files
KEEP=31
# Debug mode flag (set to 1 to enable debug mode)
DEBUG=1
# Metadata parsing flag (set to 1 to enable metadata parsing, 0 for plain text)
PARSE_METADATA=1

# Function to handle logging
log_message() {
    local message="$1"
    echo "$(date): $message" >> "$LOGFILE"
    if [ "$DEBUG" -eq 1 ]; then
        echo "$(date): $message"
    fi
}

# Ensure the log file exists
if [ ! -f "$LOGFILE" ]; then
    mkdir -p "$(dirname "$LOGFILE")"
    touch "$LOGFILE"
fi

# Check if required commands are installed (ffmpeg, curl, jq)
for cmd in ffmpeg curl jq; do
    if ! command -v $cmd &> /dev/null; then
        log_message "$cmd is not installed."
        exit 1
    fi
done

# Create recording directory if it does not exist
if [ ! -d "$RECDIR" ]; then
    mkdir -p "$RECDIR" || { log_message "Failed to create directory: $RECDIR"; exit 1; }
fi

# Remove old files based on the KEEP variable
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; || log_message "Failed to remove old files in $RECDIR"

# Check if an ffmpeg process with the specified stream URL and timestamp is already running
if pgrep -af "ffmpeg.*$STREAMURL.*$TIMESTAMP" > /dev/null; then
    log_message "An ffmpeg recording process for $TIMESTAMP with $STREAMURL is already running. Exiting."
    exit 1
fi

# Fetch current program name
if [ "$PARSE_METADATA" -eq 1 ]; then
    # Parse metadata using jq
    PROGRAM_NAME=$(curl --silent "$METADATA_URL" | jq -r '.fm.now')
    if [ -z "$PROGRAM_NAME" ] || [ "$PROGRAM_NAME" == "null" ]; then
        log_message "Failed to fetch current program name or program name is null"
        PROGRAM_NAME="Unknown Program"
    fi
else
    # Use plain value of what the URL displays
    PROGRAM_NAME=$(curl --silent "$METADATA_URL")
    if [ -z "$PROGRAM_NAME" ]; then
        log_message "Failed to fetch current program name"
        PROGRAM_NAME="Unknown Program"
    fi
fi

# Write metadata to a file
echo "$PROGRAM_NAME" > "${RECDIR}/${TIMESTAMP}.meta" || { log_message "Failed to write metadata file"; exit 1; }

# Record next hour's stream
ffmpeg -loglevel error -t 3600 -reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 300 -reconnect_on_http_error 404 -i "$STREAMURL" -c copy -f mp3 -y "${RECDIR}/${TIMESTAMP}.mp3" & disown
