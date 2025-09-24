#!/bin/bash
# This script is started by OTNS in a separate bash shell, which constitutes the process (PID)
# that OTNS will manage. If SIGTERM is sent to the process, any background processes will be
# terminated automatically - including the OT node process spawned here below.

# The preferred/requested OT node executable ($1) is determined by OTNS.
echo "runner-node.sh $@"
$1 $2 $3 $4 $5 &

while true; do
    read -p "> " cmd

    if [[ "$cmd" == "exit" ]]; then
        break
    fi

    if [[ "$cmd" == "tcat" ]]; then
        (
          cd ../openthread/tools/tcat_ble_client
          poetry run python3 bbtc.py --simulation 1 --info 2>&1
        )
        continue
    fi

    echo "hello world $cmd!"
    sleep 1
    echo "more content coming now for $cmd"
done
