#!/bin/sh
#
# OTNS node launcher wrapper for a Zephyr native_sim executable.

set -eu

NODE_ID="${1:?missing node id}"
SOCKET="${2:?missing socket path}"
SEED="${3:-}"

ZEPHYR_EXE="${ZEPHYR_EXE:?ZEPHYR_EXE must be set to the built zephyr.exe path}"

OTNS_DATA_PATH="${OTNS_DATA_PATH:?OTNS_DATA_PATH must be set by OTNS}"
PORT_OFFSET="${PORT_OFFSET:?PORT_OFFSET must be set by OTNS}"
FLASH="${OTNS_DATA_PATH}/${PORT_OFFSET}_${NODE_ID}.flash"

ENTROPY_SEED="${SEED:-$NODE_ID}"

set -- "$ZEPHYR_EXE" \
    --otns-node-id="$NODE_ID" \
    --otns-socket="$SOCKET" \
    --flash="$FLASH" \
    --seed="$ENTROPY_SEED"

if [ -n "$SEED" ]; then
    set -- "$@" --otns-seed="$SEED"
fi

exec "$@"
