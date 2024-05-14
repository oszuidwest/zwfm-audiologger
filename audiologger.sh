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

# Create recording directory if it does not exist
if [ ! -d "$RECDIR" ]; then
    mkdir -p "$RECDIR" || { log_message "Failed to create directory: $RECDIR"; exit 1; }
fi

# Remove old files based on the KEEP variable
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; || log_message "Failed to remove old files in $RECDIR"

# Kill processes from the previous hour associated with the stream URL
PIDS=$(pgrep -f "$STREAMURL")
if [ -n "$PIDS" ]; then
    kill -9 $PIDS || { log_message "Failed to kill processes: $PIDS"; exit 1; }
fi

# Fetch current program name from the API
PROGRAM_NAME=$(curl --silent "$METADATA_URL" | jq -r '.fm.now')
if [ -z "$PROGRAM_NAME" ] || [ "$PROGRAM_NAME" == "null" ]; then
    log_message "Failed to fetch current program name or program name is null"
    PROGRAM_NAME="Unknown Program"
fi

# Write metadata to a file
echo "$PROGRAM_NAME" > "${RECDIR}/${TIMESTAMP}.meta" || { log_message "Failed to write metadata file"; exit 1; }

# Record next hour's stream using curl with custom user agent
curl -s -o "${RECDIR}/${TIMESTAMP}.mp3" -A "Audiologger ZuidWest FM (2024.05)" "$STREAMURL" &> /dev/null & disown || { log_message "Failed to start recording from $STREAMURL"; exit 1; }
