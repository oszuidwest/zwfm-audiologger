#!/bin/bash
# Records hourly segments from a live stream and stores program metadata
# Designed to be run via cron every hour

CONFIG_FILE=${CONFIG_FILE:-'/app/streams.json'}

# Check if config exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "ERROR: Config file not found: $CONFIG_FILE"
    exit 1
fi

# Load global settings
RECDIR=$(jq -r '.global.rec_dir' "$CONFIG_FILE")
LOGFILE=$(jq -r '.global.log_file' "$CONFIG_FILE")
DEBUG=$(jq -r '.global.debug' "$CONFIG_FILE")
KEEP=$(jq -r '.global.keep_days' "$CONFIG_FILE")

# Setup logging
mkdir -p "$(dirname "$LOGFILE")"
touch "$LOGFILE"

# Log with timestamps
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S'): $1" >> "$LOGFILE"
    [[ $DEBUG -eq 1 ]] && echo "$(date '+%Y-%m-%d %H:%M:%S'): $1"
}

# Function to clean old files asynchronously
cleanup_files() {
    local dir=$1
    local days=$2
    local name=$3
    {
        # Calculate the number of minutes based on the days in the configuration
        local minutes=$((days * 1440))
        
        find "$dir" -type f -mmin +"$minutes" -print0 | xargs -0 -r rm 2>/dev/null || \
            log "WARNING: Failed to cleanup old files for $name"
        log "INFO: Completed cleanup for $name"
    } &
}

# Function to fetch and store metadata
fetch_metadata() {
    local name=$1
    local metadata_url=$2
    local metadata_path=$3
    local parse_metadata=$4
    local stream_dir=$5
    local timestamp=$6

    # Get program info
    {
        if [[ $parse_metadata -eq 1 && -n "$metadata_path" ]]; then
            PROGRAM_NAME=$(curl -s --max-time 15 "$metadata_url" 2>/dev/null | jq -r "$metadata_path")
        else
            PROGRAM_NAME=$(curl -s --max-time 15 "$metadata_url" 2>/dev/null)
        fi
        
        [[ -z "$PROGRAM_NAME" || "$PROGRAM_NAME" == "null" ]] && PROGRAM_NAME="Unknown Program"
        
        # Store metadata
        echo "$PROGRAM_NAME" > "${stream_dir}/${timestamp}.meta" || \
            log "WARNING: Failed to write metadata for $name"
        
        log "INFO: Stored metadata for $name - $timestamp - $PROGRAM_NAME"
    } &
}

# Check dependencies
for cmd in ffmpeg curl jq xargs; do
    if ! command -v $cmd &> /dev/null; then
        log "ERROR: $cmd is not installed."
        exit 1
    fi
done

# Create base directory
mkdir -p "$RECDIR"
TIMESTAMP=$(date +"%Y-%m-%d_%H")

# Process each stream
while read -r stream_base64; do
    # Decode stream config
    stream_json=$(echo "$stream_base64" | base64 -d)
    name=$(echo "$stream_json" | jq -r '.key')
    stream_url=$(echo "$stream_json" | jq -r '.value.stream_url')
    metadata_url=$(echo "$stream_json" | jq -r '.value.metadata_url')
    metadata_path=$(echo "$stream_json" | jq -r '.value.metadata_path // empty')
    parse_metadata=$(echo "$stream_json" | jq -r '.value.parse_metadata // 0')
    keep_days=$(echo "$stream_json" | jq -r '.value.keep_days // empty')
    
    # Use stream-specific keep_days or fall back to global KEEP
    [[ -z "$keep_days" ]] && keep_days=$KEEP
    
    # Create stream directory
    stream_dir="$RECDIR/$name"
    mkdir -p "$stream_dir"
    
    # Start async cleanup
    cleanup_files "$stream_dir" "$keep_days" "$name"
    
    # Check for existing recording
    if pgrep -af "ffmpeg.*$stream_url.*$TIMESTAMP" > /dev/null; then
        log "WARNING: Recording for $name $TIMESTAMP already running"
        continue
    fi
    
    # Start metadata fetch in background
    fetch_metadata "$name" "$metadata_url" "$metadata_path" "$parse_metadata" "$stream_dir" "$TIMESTAMP"
    
    # Start recording
    log "INFO: Starting recording for $name - $TIMESTAMP"
    ffmpeg -nostdin -loglevel error \
        -t 3600 \
        -reconnect 1 \
        -reconnect_at_eof 1 \
        -reconnect_streamed 1 \
        -reconnect_delay_max 300 \
        -reconnect_on_http_error 404,500,503 \
        -rw_timeout 10000000 \
        -i "$stream_url" \
        -c copy \
        -f mp3 \
        -y "${stream_dir}/${TIMESTAMP}.mp3" 2>> "$LOGFILE" & disown

done < <(jq -r '.streams | to_entries[] | @base64' "$CONFIG_FILE")