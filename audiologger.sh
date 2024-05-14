#!/bin/bash

# Configuration
STREAMURL='https://icecast.zuidwestfm.nl/zuidwest.mp3'
RECDIR='/var/audio'
LOGFILE='/var/log/audiologger.log'
METADATA_URL='https://www.zuidwestupdate.nl/wp-json/zw/v1/broadcast_data'
# Output date and hour, e.g., "2023_12_31_20u"
TIMESTAMP=$(/bin/date +"%Y-%m-%d_%Hu")
# Number of days to keep the audio files
KEEP=31

# Check if jq and wget are installed
if ! command -v jq &> /dev/null; then
    echo "$(date): jq is not installed" >> "$LOGFILE"
    exit 1
fi

if ! command -v wget &> /dev/null; then
    echo "$(date): wget is not installed" >> "$LOGFILE"
    exit 1
fi

# Create recording directory if it does not exist
if [ ! -d "$RECDIR" ]; then
    mkdir -p "$RECDIR" || { echo "$(date): Failed to create directory: $RECDIR" >> "$LOGFILE"; exit 1; }
fi

# Remove old files based on the KEEP variable
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; || { echo "$(date): Failed to remove old files in $RECDIR" >> "$LOGFILE"; exit 1; }

# Kill processes from the previous hour associated with the stream URL
PIDS=$(pgrep -f "$STREAMURL")
if [ -n "$PIDS" ]; then
    kill -9 $PIDS || { echo "$(date): Failed to kill processes: $PIDS" >> "$LOGFILE"; exit 1; }
fi

# Fetch current program name from the API
PROGRAM_NAME=$(curl --silent "$METADATA_URL" | jq -r '.fm.now')
if [ -z "$PROGRAM_NAME" ] || [ "$PROGRAM_NAME" == "null" ]; then
    echo "$(date): Failed to fetch current program name or program name is null" >> "$LOGFILE"
    PROGRAM_NAME="Unknown Program"
fi

# Write metadata to a file
echo "$PROGRAM_NAME" > "${RECDIR}/${TIMESTAMP}.metadata" || { echo "$(date): Failed to write metadata file" >> "$LOGFILE"; exit 1; }

# Record next hour's stream
wget --quiet --background --user-agent="Audiologger ZuidWest (2024.05)" -O "${RECDIR}/${TIMESTAMP}.mp3" "$STREAMURL" > /dev/null 2>&1 || { echo "$(date): Failed to start recording from $STREAMURL" >> "$LOGFILE"; exit 1; }