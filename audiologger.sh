#!/usr/bin/env bash
#
# Records hourly segments from a live stream and stores program metadata.
# Designed to be run via cron every hour.

set -euo pipefail

###############################################################################
# CONFIGURATION & DEFAULTS
###############################################################################

CONFIG_FILE=${CONFIG_FILE:-'streams.json'}

# We'll use a "GLOBAL" label for anything that doesn't pertain to a specific station.
GLOBAL_LABEL="GLOBAL"

# Check if the configuration file exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "ERROR: Config file not found: $CONFIG_FILE"
    exit 1
fi

# Load global settings
RECDIR=$(jq -r '.global.rec_dir // empty' "$CONFIG_FILE")
LOGFILE=$(jq -r '.global.log_file // empty' "$CONFIG_FILE")
DEBUG_VAL=$(jq -r '.global.debug // "0"' "$CONFIG_FILE")  # Should be 0 or 1
KEEP=$(jq -r '.global.keep_days // 7' "$CONFIG_FILE")     # Default to 7 days if not defined

# Validate that RECDIR and LOGFILE were actually loaded from the config
if [[ -z "$RECDIR" ]]; then
    echo "ERROR: 'rec_dir' is not specified in $CONFIG_FILE"
    exit 1
fi

if [[ -z "$LOGFILE" ]]; then
    echo "ERROR: 'log_file' is not specified in $CONFIG_FILE"
    exit 1
fi

# DEBUG is expected to be either 0 or 1; if not numeric, default to 0
if [[ "$DEBUG_VAL" =~ ^[0-9]+$ ]]; then
    DEBUG=$DEBUG_VAL
else
    DEBUG=0
fi

# Prepare the log file
mkdir -p "$(dirname "$LOGFILE")"
touch "$LOGFILE"

###############################################################################
# HELPER FUNCTIONS
###############################################################################

# Universal logging function with station prefix
log() {
    local level="$1"
    local station="$2"
    local msg="$3"

    # If station is missing or empty, set it to GLOBAL
    [[ -z "$station" ]] && station="$GLOBAL_LABEL"

    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    echo "${timestamp} [${level}] [${station}] ${msg}" >> "$LOGFILE"

    # Only echo to console if DEBUG != 0
    if [[ "$DEBUG" -ne 0 ]]; then
        echo "${timestamp} [${level}] [${station}] ${msg}"
    fi
}

# Asynchronously clean up old files
cleanup_files() {
    local dir="$1"
    local days="$2"
    local station="$3"
    {
        local minutes=$((days * 1440))
        if find "$dir" -type f -mmin +"$minutes" -print0 2>/dev/null | xargs -0 -r rm 2>/dev/null; then
            log "INFO" "$station" "Completed cleanup"
        else
            log "WARNING" "$station" "Failed to clean up old files"
        fi
    } >>"$LOGFILE" 2>&1 &
}

# Fetch and store metadata
fetch_metadata() {
    local station="$1"
    local metadata_url="$2"
    local metadata_path="$3"
    local parse_metadata="$4"
    local stream_dir="$5"
    local timestamp="$6"
    
    {
        local program_name="Unknown Program"
        local curl_out

        # Fetch data from metadata_url (with a 15s timeout)
        curl_out="$(curl -s --max-time 15 "$metadata_url" 2>/dev/null || true)"

        if [[ -n "$curl_out" ]]; then
            if [[ "$parse_metadata" -eq 1 && -n "$metadata_path" ]]; then
                program_name="$(echo "$curl_out" | jq -r "$metadata_path" 2>/dev/null || echo "Unknown Program")"
            else
                program_name="$curl_out"
            fi
        fi
        
        [[ -z "$program_name" || "$program_name" == "null" ]] && program_name="Unknown Program"

        # Write to .meta file
        if ! echo "$program_name" > "${stream_dir}/${timestamp}.meta"; then
            log "WARNING" "$station" "Failed to write metadata"
        fi
        
        log "INFO" "$station" "Stored metadata - ${timestamp} - ${program_name}"
    } >>"$LOGFILE" 2>&1 &
}

###############################################################################
# VALIDATE REQUIRED COMMANDS
###############################################################################

for cmd in ffmpeg curl jq xargs; do
    if ! command -v "$cmd" &>/dev/null; then
        log "ERROR" "$GLOBAL_LABEL" "${cmd} is not installed or not in the PATH."
        exit 1
    fi
done

###############################################################################
# MAIN LOGIC
###############################################################################

# Make sure the recording directory exists
mkdir -p "$RECDIR"
TIMESTAMP=$(date +"%Y-%m-%d_%H")

# Iterate over each stream
while read -r stream_base64; do
    # Decode base64 -> JSON
    stream_json="$(echo "$stream_base64" | base64 -d)"
    
    # Read fields
    name=$(echo "$stream_json" | jq -r '.key')
    stream_url=$(echo "$stream_json" | jq -r '.value.stream_url')
    metadata_url=$(echo "$stream_json" | jq -r '.value.metadata_url')
    metadata_path=$(echo "$stream_json" | jq -r '.value.metadata_path // empty')
    parse_metadata=$(echo "$stream_json" | jq -r '.value.parse_metadata // 0')
    keep_days=$(echo "$stream_json" | jq -r '.value.keep_days // empty')

    # Use global KEEP if keep_days is not set for this stream
    [[ -z "$keep_days" ]] && keep_days="$KEEP"
    
    # Verify critical data
    if [[ -z "$name" || -z "$stream_url" ]]; then
        log "WARNING" "$GLOBAL_LABEL" "Skipping a stream with invalid config"
        continue
    fi

    # Prepare stream directory
    stream_dir="$RECDIR/$name"
    mkdir -p "$stream_dir"
    
    # Clean up old files asynchronously
    cleanup_files "$stream_dir" "$keep_days" "$name"
    
    # Check if a recording is already running for this stream
    if pgrep -af "ffmpeg.*${stream_url}.*${TIMESTAMP}" &>/dev/null; then
        log "WARNING" "$name" "Recording for this station is already running"
        continue
    fi
    
    # Fetch metadata asynchronously
    fetch_metadata "$name" "$metadata_url" "$metadata_path" "$parse_metadata" "$stream_dir" "$TIMESTAMP"

    # Start ffmpeg in the background and capture output
    log "INFO" "$name" "Starting recording - ${TIMESTAMP}"
    (
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
            -y "${stream_dir}/${TIMESTAMP}.mp3"
    ) 2>&1 | while IFS= read -r line; do
        log "INFO" "$name" "$line"
    done & disown

done < <(jq -r '.streams | to_entries[] | @base64' "$CONFIG_FILE")

exit 0
