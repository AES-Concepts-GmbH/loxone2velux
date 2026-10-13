#!/bin/sh
# ==============================================================================
# Loxone2Velux Gateway - Startup script
# Reads HA add-on options from /data/options.json via jq
# Config at /config/loxone2velux/config.yaml
# ==============================================================================

echo "=== Loxone2Velux Gateway starting ==="

# Read add-on options from Home Assistant
OPTIONS_FILE="/data/options.json"
CONFIG_DIR="/config/loxone2velux"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
LISTEN_PORT=8099

# opt_str NAME DEFAULT  -> JSON-quoted string (valid as a YAML double-quoted scalar,
#                           so passwords with quotes, $ or backslashes are safe)
# opt_num NAME DEFAULT  -> plain number
opt_str() {
    if [ -f "$OPTIONS_FILE" ]; then
        jq --arg d "$2" -r ".$1 // \$d | tostring | @json" "$OPTIONS_FILE" 2>/dev/null && return
    fi
    printf '"%s"\n' "$2"
}

opt_num() {
    if [ -f "$OPTIONS_FILE" ]; then
        v=$(jq -r ".$1 // $2" "$OPTIONS_FILE" 2>/dev/null)
        case "$v" in
            ''|*[!0-9]*) ;;  # not a plain integer -> default
            *) echo "$v"; return ;;
        esac
    fi
    echo "$2"
}

if [ ! -f "$OPTIONS_FILE" ]; then
    echo "WARNING: Options file not found at ${OPTIONS_FILE}, using defaults"
fi

KLF200_HOST=$(opt_str klf200_host "")
KLF200_PASSWORD=$(opt_str klf200_password "")
KLF200_PORT=$(opt_num klf200_port 51200)
RECONNECT_INTERVAL=$(opt_num reconnect_interval 30)
REFRESH_INTERVAL=$(opt_num refresh_interval 300)
LOG_LEVEL=$(opt_str log_level "info")
API_TOKEN=$(opt_str api_token "")

echo "KLF-200 host: $(printf %s "$KLF200_HOST" | sed 's/^"//;s/"$//'):${KLF200_PORT}"
echo "Server listening on: 0.0.0.0:${LISTEN_PORT}"
if [ "$API_TOKEN" = '""' ]; then
    echo "WARNING: api_token is empty - the REST API on port ${LISTEN_PORT} is reachable without authentication"
fi

# Always (re)generate config from HA options to avoid stale/corrupt state.
# Mappings and UDP feedback live in loxone.yaml next to it and are not touched.
mkdir -p "${CONFIG_DIR}"
umask 077
cat > "${CONFIG_FILE}" << EOF
klf200:
  host: ${KLF200_HOST}
  port: ${KLF200_PORT}
  password: ${KLF200_PASSWORD}
  reconnect_interval: ${RECONNECT_INTERVAL}s
  refresh_interval: ${REFRESH_INTERVAL}s

server:
  host: "0.0.0.0"
  port: ${LISTEN_PORT}
  read_timeout: 15s
  write_timeout: 15s
  api_token: ${API_TOKEN}

logging:
  level: ${LOG_LEVEL}
  format: "console"
EOF

echo "Configuration written to ${CONFIG_FILE}"

# Change to /app so the binary can find web/dist
cd /app || true

echo "Starting binary..."

# Start the gateway (exec replaces shell with Go binary)
exec /usr/bin/loxone2velux -config "${CONFIG_FILE}"
