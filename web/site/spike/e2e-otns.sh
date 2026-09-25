#!/bin/bash
# End-to-end check of the web visualizer against a live otns: starts otns, grpcwebproxy and a
# static web server for page variants in $E2E_DIR/site, then screenshots each variant named on the
# command line with headless-run.mjs (see README.md for the layout of $E2E_DIR).
#   E2E_DIR   scratch directory (default /tmp/otns-e2e) with site/visualize-<variant>.html,
#             site/static/js/visualize-<variant>.js, site/static/image/ (copy of web/site/static/image),
#             cmds.txt (otns CLI commands), optional done.js and action.js (see headless-run.mjs)
#   OTNS_BIN  otns executable (default: otns on PATH)
# Ports: otns -listen localhost:9100 (gRPC 9099, grpcwebproxy 9098), pages on 9150.
E=${E2E_DIR:-/tmp/otns-e2e}
REPO=$(cd "$(dirname "$0")/../../.." && pwd)
cd $REPO || exit 1
pkill -f "otns-bin/otns" 2>/dev/null; pkill -f "grpcwebproxy.*9098" 2>/dev/null; pkill -f "http.server 9150" 2>/dev/null
rm -rf $E/out; mkdir -p $E/out
# otns keeps running while its stdin stays open
( cat $E/cmds.txt; sleep 150 ) | ${OTNS_BIN:-otns} -web=false -speed 20 -listen localhost:9100 -output $E/out -log warn > $E/otns.log 2>&1 &
OTNS_PID=$!
grpcwebproxy --backend_addr=localhost:9099 --run_tls_server=false --allow_all_origins \
    --server_http_max_read_timeout=1h --server_http_max_write_timeout=1h \
    --server_bind_address=localhost --server_http_debug_port=9098 > $E/proxy.log 2>&1 &
PROXY_PID=$!
( cd $E/site && python3 -m http.server 9150 --bind 127.0.0.1 > $E/http.log 2>&1 ) &
HTTP_PID=$!
sleep 12  # let the nodes form a network at 20x speed
for v in "$@"; do
  unset ACTION_EXPR DONE_EXPR
  [ -f $E/done.js ] && DONE_EXPR=$(cat $E/done.js)
  [ -f $E/done-$v.js ] && DONE_EXPR=$(cat $E/done-$v.js)
  [ -f $E/action.js ] && ACTION_EXPR=$(cat $E/action.js)
  [ -f $E/action-$v.js ] && ACTION_EXPR=$(cat $E/action-$v.js)
  export ACTION_EXPR
  DONE_EXPR=${DONE_EXPR:-'(window.otnsVis && Object.keys(otnsVis.nodes).length >= 6 && Object.values(otnsVis.nodes).every(n => n.rloc16 !== 0xfffe && n.role !== 0)) ? (otnsVis.showLogWindow(), otnsVis.setSelectedNode(2), true) : false'} \
  SETTLE_MS=1500 HEADLESS_PROFILE=$E node $REPO/web/site/spike/headless-run.mjs "http://localhost:9150/visualize-$v.html" $E/shot-$v.png 60 > $E/run-$v.log 2>&1
  echo "$v: exit=$? $(grep -c . $E/run-$v.log) log lines; $(grep -ci 'error\|exception' $E/run-$v.log) errors"
done
kill $OTNS_PID $PROXY_PID $HTTP_PID 2>/dev/null; pkill -f "otns-bin/otns" 2>/dev/null; pkill -f "http.server 9150" 2>/dev/null
wait 2>/dev/null
echo "otns log tail:"; tail -3 $E/otns.log
