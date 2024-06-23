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
DEBUG=0
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

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    log_message "jq is not installed"
    exit 1
fi

# Check if ffmpeg is installed
if ! command -v ffmpeg &> /dev/null; then
    log_message "ffmpeg is not installed"
    exit 1
fi

# Create recording directory if it does not exist
if [ ! -d "$RECDIR" ]; then
    mkdir -p "$RECDIR" || { log_message "Failed to create directory: $RECDIR"; exit 1; }
fi

# Remove old files based on the KEEP variable
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; || log_message "Failed to remove old files in $RECDIR"

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
ffmpeg -loglevel error -t 3600 -reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 -i "$STREAMURL" -c copy -f mp3 -y "${RECDIR}/${TIMESTAMP}.mp3" & disown
