#!/bin/bash
# ot-br-ncp.sh - to launch a simulated Docker OT Border Router (NCP side only) from OT-NS.
# The NCP in the docker container is connected to an RCP via a TTY (PTY) device.
# An instance of 'socat' is started to expose Docker's TTY USB on the host machine, as PTY, where the RCP
# is running. The PTY will have a known name PTY_FILE.
# See for more info: https://openthread.io/guides/border-router
#
# This script starts the ot-ctl CLI terminal that connects to the OTBR NCP.

# Cmdline parameters provided by OT-NS:
# $1  <node>    Node number in the simulation
# $2  <socket>  File for the Unix domain socket for OT-NS communication.
# $3  <simNum>  Simulation instance number (default 0, may be 1, 2, 3 etc)
# $4  <ptyFile> The absolute file path that OTNS expects for the PTY device (or a symlink to it).
#               The PTY file (symlink) path will be in a temp folder as created by OTNS.

set -u

if [[ $# -ne 4 ]]; then
    echo "[C] ERROR: ob-br-ncp.sh requires 4 arguments; not for interactive use."
    exit 1
fi

CONTAINER_NAME="otns_otbr_${3}_${1}"

echo "[D] ot-br-ncp.sh started - $3_$1  simid=$3  node=$1  socket=$2"

_term()
{
    echo "[D] Received SIGTERM"
    docker kill "${CONTAINER_NAME}" >&/dev/null
    exit 0
}
trap _term SIGTERM

echo "[D] Waiting for Docker container ${CONTAINER_NAME}"
sleep 3

echo "[D] Starting ot-ctl CLI on Docker container ${CONTAINER_NAME}"
docker exec -i "${CONTAINER_NAME}" /bin/bash -c "until [ -e /dev/wpan0 ];do sleep 0.2;done; ot-ctl"

echo "[D] script exit"
