#!/bin/bash

# Configuration
STREAMURL='https://icecast.zuidwestfm.nl/zuidwest.mp3'
RECDIR='/var/audio'
LOGFILE='/var/log/audiologger.log'
METADATA_URL='https://www.zuidwestupdate.nl/wp-json/zw/v1/broadcast_data'
KEEP=31
DEBUG=1
PARSE_METADATA=1

# Initialize logging
mkdir -p "$(dirname "$LOGFILE")"
touch "$LOGFILE"

# Log function
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S'): $1" >> "$LOGFILE"
    [[ $DEBUG -eq 1 ]] && echo "$(date '+%Y-%m-%d %H:%M:%S'): $1"
}

# Check requirements
for cmd in ffmpeg curl jq; do
    if ! command -v $cmd &> /dev/null; then
        log "ERROR: $cmd is not installed."
        exit 1
    fi
done

# Cleanup old files and ensure record directory exists
mkdir -p "$RECDIR"
find "$RECDIR" -type f -mtime "+$KEEP" -exec rm {} \; 2>/dev/null || log "WARNING: Failed to cleanup old files"

# Get timestamp
TIMESTAMP=$(date +"%Y-%m-%d_%H")

# Check for running process
if pgrep -af "ffmpeg.*$STREAMURL.*$TIMESTAMP" > /dev/null; then
    log "WARNING: Recording for $TIMESTAMP already running"
    exit 1
fi

# Get program name
if [[ $PARSE_METADATA -eq 1 ]]; then
    PROGRAM_NAME=$(curl -s --max-time 5 "$METADATA_URL" 2>/dev/null | jq -r '.fm.now')
    [[ -z "$PROGRAM_NAME" || "$PROGRAM_NAME" == "null" ]] && PROGRAM_NAME="Unknown Program"
else
    PROGRAM_NAME=$(curl -s --max-time 5 "$METADATA_URL" 2>/dev/null)
    [[ -z "$PROGRAM_NAME" ]] && PROGRAM_NAME="Unknown Program"
fi

# Save metadata
echo "$PROGRAM_NAME" > "${RECDIR}/${TIMESTAMP}.meta" || log "WARNING: Failed to write metadata"

# Start recording
log "INFO: Starting recording for $TIMESTAMP - $PROGRAM_NAME"
ffmpeg -loglevel error \
    -t 3600 \
    -reconnect 1 \
    -reconnect_at_eof 1 \
    -reconnect_streamed 1 \
    -reconnect_delay_max 300 \
    -reconnect_on_http_error 404,500,503 \
    -rw_timeout 10000000 \
    -i "$STREAMURL" \
    -c copy \
    -f mp3 \
    -y "${RECDIR}/${TIMESTAMP}.mp3" 2>> "$LOGFILE" & disown