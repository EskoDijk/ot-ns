# OTNS Godot client

A Godot 4 project that shows a running OTNS simulation in 3D: it connects to the OTNS gRPC
event stream through the gRPC-web proxy that OTNS starts for its web UI, and draws the nodes,
links and messages. It is the starting point for the game-like views discussed in
`studies/external-engine-interface.md` (luminaires in a building or a street scene).

## Run

1. Start OTNS as usual (the web UI may stay open; both clients receive the same events):

   ```
   otns -floorplan etc/floorplans/bistro.json
   ```

2. Open this folder as a project in Godot 4.7 (or newer) and run it, or from the command line:

   ```
   godot4 --path etc/godot-client
   ```

   The client connects to `127.0.0.1:8998`, the gRPC-web port of an OTNS started with the
   default `-listen localhost:9000` (proxy port = base − 2; use the IPv4 address, the proxy
   does not listen on `::1`). Change host and port on the `OtnsClient` node of the demo scene
   or pass `--otns=host:port` after `--`:

   ```
   godot4 --path etc/godot-client -- --otns=127.0.0.1:9098
   ```

## Layout

- `addons/otns_client/`: the reusable part, to copy into another Godot project.
  - `otns_grpc_web.gd`: gRPC-web client over Godot's `HTTPClient`: opens the server-streaming
    `Visualize` call, parses the gRPC-web frames and decodes each `VisualizeEvent`; also the
    unary `Command` call for sending CLI commands.
  - `otns_client.gd` (`OtnsClient` node): connection, reconnect, the node state table (ID,
    type, role, position, parent, tables) and signals per event; converts OTNS positions to
    Godot with the scale and origin of a floor plan (`unitsPerMeter`, `origin`).
  - `otns_field.gd` (`OtnsField` node): a default drawing of the state: a sphere per node
    colored by role (the thread skin palette), a label, links as lines, messages as sparks.
  - `proto/visualize_grpc_pb.gd`: the OTNS protobuf messages as GDScript, generated with
    [godobuf](https://github.com/oniksan/godobuf) (BSD-3) from
    `visualize/grpc/pb/visualize_grpc.proto` (messages only; godobuf does not parse the
    `service` block). Regenerate after a proto change:

    ```
    sed '/^service VisualizeGrpcService {/,/^}/d' visualize/grpc/pb/visualize_grpc.proto > /tmp/visualize_grpc.proto
    godot4 --headless --path <godobuf project> -s addons/godobuf/godobuf_cmdln.gd \
        --input=/tmp/visualize_grpc.proto --output=<this dir>/addons/otns_client/proto/visualize_grpc_pb.gd
    ```
- `demo/`: a scene with an `OtnsClient`, an `OtnsField`, a free-flying camera and a status line.
- `tools/stream_test.gd`: headless check, prints the first events of a running OTNS and exits:

  ```
  godot4 --headless --path etc/godot-client -s tools/stream_test.gd -- --otns=127.0.0.1:8998 --events=20
  ```

## Notes

- gRPC-web over HTTP/1.1 supports server streaming, which is all the visualizer needs. Godot's
  `HTTPClient` has no HTTP/2, so the native gRPC port (base − 1) cannot be used directly.
- The stream resends the full state on connect, so the client can start or reconnect at any
  time.
- Coordinates: OTNS (x, y, z) with z up map to Godot (x, z, y) after the floor plan's scale and
  origin, the same mapping the web visualizer's 3D skin uses.
