#!/bin/bash

# Configuration
STREAMURL='https://icecast.zuidwestfm.nl/zuidwest.mp3'
RECDIR='/var/audio'
LOGFILE='/var/log/audiologger.log'
# Output date and hour, e.g., "2023_12_31_20u"
TIMESTAMP=$(/bin/date +"%Y-%m-%d_%Hu")
# Number of days to keep the audio files
KEEP=31

# Create recording directory if it does not exist
if [ ! -d "$RECDIR" ]; then
    mkdir -p "$RECDIR" || { echo "$(date): Failed to create directory: $RECDIR" >> "$LOGFILE"; exit 1; }
fi

# Remove old files based on the KEEP variable
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; || { echo "$(date): Failed to remove old files in $RECDIR" >> "$LOGFILE"; exit 1; }

# Kill processes from the previous hour associated with the stream URL
pids=$(pgrep -f "$STREAMURL")
if [ -n "$pids" ]; then
    kill -9 $pids || { echo "$(date): Failed to kill processes: $pids" >> "$LOGFILE"; exit 1; }
fi

# Record next hour's stream
wget --quiet --background --user-agent="Audiologger ZuidWest (Debian 11)" -O "${RECDIR}/${TIMESTAMP}.mp3" "$STREAMURL" > /dev/null 2>&1 || { echo "$(date): Failed to start recording from $STREAMURL" >> "$LOGFILE"; exit 1; }
