#!/bin/sh
# Generate coturn config from the environment and exec turnserver.
# Compose sets TURN_* from TELLOMI_TURN_* / TELLOMI_CALL_IP. See docs/calls.md.
set -eu

: "${TURN_EXTERNAL_IP:=127.0.0.1}"
: "${TURN_SECRET:=tellomi-turn-secret}"
: "${TURN_REALM:=tellomi}"
: "${TURN_MIN_PORT:=49160}"
: "${TURN_MAX_PORT:=49200}"
: "${TURN_LISTEN_PORT:=3478}"
: "${TURN_TLS_PORT:=5349}"

# TELLOMI_TURN_PORT=0 tells the bff to omit TURN candidates. coturn still
# needs a real listener if the container is started.
if [ "$TURN_LISTEN_PORT" = "0" ]; then
  TURN_LISTEN_PORT=3478
fi

external="$TURN_EXTERNAL_IP"
listen_line="listening-ip=0.0.0.0"
relay_line="relay-ip=0.0.0.0"
if [ -n "${TURN_RELAY_IP:-}" ]; then
  # Public EIP in front of a private NIC. coturn rewrites relay candidates
  # from the private address to the public one.
  external="${TURN_EXTERNAL_IP}/${TURN_RELAY_IP}"
  listen_line="listening-ip=${TURN_RELAY_IP}"
  relay_line="relay-ip=${TURN_RELAY_IP}"
fi

tls_line=""
if [ "$TURN_TLS_PORT" != "0" ]; then
  tls_line="tls-listening-port=${TURN_TLS_PORT}"
fi

conf=/tmp/turnserver.conf
cat > "$conf" <<EOF
listening-port=${TURN_LISTEN_PORT}
${tls_line}
${listen_line}
${relay_line}
external-ip=${external}
min-port=${TURN_MIN_PORT}
max-port=${TURN_MAX_PORT}
fingerprint
lt-cred-mech
use-auth-secret
static-auth-secret=${TURN_SECRET}
realm=${TURN_REALM}
no-multicast-peers
no-cli
log-file=stdout
simple-log
EOF

exec turnserver -c "$conf"
