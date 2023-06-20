#!/bin/bash
# ot-br-ncp.sh - to launch a simulated Docker OT Border Router (NCP side only) from OT-NS.
# The NCP in the docker container is connected to an RCP via a TTY (PTY) device.
# An instance of 'socat' is started to expose Docker's TTY USB on the host machine, as PTY, where the RCP
# is running. The PTY will have a known name PTY_FILE.
# See for more info: https://openthread.io/guides/border-router
#
# Cmdline parameters provided by OT-NS:
# $1  <node>    Node number in the simulation
# $2  <socket>  File for the Unix domain socket for OT-NS communication.
# $3  <simNum>  Simulation instance number (default 0, may be 1, 2, 3 etc)
# $4  <ptyFile> The absolute file path that OTNS expects for the PTY device (or a symlink to it).
#               The PTY file (symlink) path will be in a temp folder as created by OTNS.

if [[ $# -ne 4 ]]; then
    echo "ERROR: ob-br-ncp.sh requires 4 arguments; not for interactive use."
    exit 1
fi

SOCAT_PID=""
PTY_FILE="${4}"
PTY_FILE2="${PTY_FILE}d"
CONTAINER_NAME="otns_otbr_${3}_${1}"
WEB_PORT="$((8080 + ${1} + ${3} * 1000))"

echo "ot-br-ncp.sh started - simid=$3  node=$1  socketfile=$2"
echo "                       ptyFile=$PTY_FILE  ptyFileDocker=$PTY_FILE2"
echo "                       webPort=${WEB_PORT}"

_term()
{
    echo " - Received SIGTERM! Killing SOCAT_PID ($SOCAT_PID) and docker kill ${CONTAINER_NAME}."
    kill $SOCAT_PID
    docker kill ${CONTAINER_NAME}
    wait
    exit 0
}
trap _term SIGTERM

echo " - starting 'socat' to connect OT-NS pty to Docker pty"
socat -d pty,raw,echo=0,link=$PTY_FILE pty,raw,echo=0,link=$PTY_FILE2 &
SOCAT_PID=$!

echo " - starting docker container '${CONTAINER_NAME}' for OT-BR NCP side"
# https://docs.docker.com/engine/reference/run/
# -t flag must not be used when stdinput is piped to this script. So -it becomes -i
# --rm flag to remove container after exit to avoid pollution of Docker data. Remove this for post mortem debug.
# --entrypoint overrides the default otbr docker startup script - non-trivial to use see docs.
# -c provides cmd arguments for the 'entrypoint' executable.
# sed pipe prepends a log string to each line coming from docker.
docker run --name ${CONTAINER_NAME} \
    --sysctl "net.ipv6.conf.all.disable_ipv6=0 net.ipv4.conf.all.forwarding=1 net.ipv6.conf.all.forwarding=1" \
    -p ${WEB_PORT}:80 --dns=127.0.0.1 -i --rm --volume $PTY_FILE2:/dev/ttyUSB0 --privileged \
    --entrypoint /bin/bash \
    openthread/otbr \
    -c "/app/etc/docker/docker_entrypoint.sh" \
    | sed -E 's/^/[L] /' &

# Wait for 'wpan0' device to appear
#docker exec -i ${CONTAINER_NAME} /bin/bash -c "ls /dev/wpan0"
#while [ $? -ne 0 ]
#do
#  docker exec -i ${CONTAINER_NAME} /bin/bash -c "ls /dev/wpan0"
#done

# Run OT CLI in foreground until 'exit' typed or SIGTERM sent to this script.
sleep 5
docker exec -i ${CONTAINER_NAME} ot-ctl

echo " - CLI exited, stopping docker and killing 'socat' process"
docker kill ${CONTAINER_NAME}
kill $SOCAT_PID
wait

echo " - script exit"
exit 0
