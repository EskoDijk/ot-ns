#!/bin/bash
# run-docker-ot-ctl.sh - script to start ot-ctl CLI inside a given docker container that is
# already running. The NCP docker container is connected to an RCP via a TTY (PTY) device.
# See for more info: https://openthread.io/guides/border-router
#
# The script waits until the /dev/wpan0 device becomes available which signals that Thread
# is active and ot-ctl can be run.
#
# Cmdline parameters provided by OT-NS:
# $1  <CONTAINER_NAME>    Name of the NCP Docker container

set -u

if [[ $# -ne 1 ]]; then
    echo "[C] ERROR: run-docker-ot-ctl.sh requires 1 argument CONTAINER_NAME"
    exit 1
fi

CONTAINER_NAME="${1}"

_term()
{
    echo "[D] run-docker-ot-ctl.sh received SIGTERM"
    exit 0
}
trap _term SIGTERM

echo "[D] Starting wpan0 monitor and ot-ctl CLI on Docker container ${CONTAINER_NAME}"
docker exec -i "${CONTAINER_NAME}" /bin/bash -c "until [ -e /sys/devices/virtual/net/wpan0 ];do sleep 0.2;done; echo '[D] Thread wpan0 device found' ; sleep 0.4; ot-ctl"

echo "[D] run-docker-ot-ctl.sh script regular exit"
exit 0
