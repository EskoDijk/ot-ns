#!/bin/bash
# test script for OT-BR NCP CLI communication with OT-NS.
# This gets Stdin input lines, prints something back as if the command was executed, and
# exits when the user types 'exit'.

if [[ $# -ne 4 ]]; then
    echo "ERROR: ob-br-test.sh requires 4 arguments."
    exit 1
fi

CONTAINER_NAME="otns_otbr_${3}_${1}"

echo "ot-br-test.sh started - simid=$3  node=$1  socketfile=$2"

_term()
{
    #docker kill ${CONTAINER_NAME}
    wait
    exit 0
}
trap _term SIGTERM

echo " - starting docker container '${CONTAINER_NAME}' for testing OT-BR NCP side"
docker run --name ${CONTAINER_NAME} -i --rm --entrypoint /bin/bash openthread/otbr -c "sleep 5"

while read -p ">" INPUT
do
  echo "$INPUT"
  echo "This is fake output for your '$INPUT' command. Hello there!"
  echo "Done"
  if [ "$INPUT" == "exit" ]; then
    exit 0
  fi
done

echo " - script exit"
exit 0
