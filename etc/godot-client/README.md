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
  - `otns_luminaires.gd` (`OtnsLuminaires` node): drives the lights of an existing scene from
    OTNS node state through a JSON mapping of node ID to scene node (`etc/floorplans/
    bistro-lights.json` for the Bistro demo). A failed node (radio off) always switches its
    light off; otherwise the light is on while the node is attached (Child, Router, Leader) and
    off when detached or disabled. Lights (`Light3D`) are switched by visibility; emissive
    meshes get a duplicated material with emission on or off and a darkened albedo when off,
    following the scene's own day/night switching of the shared material. A mapping entry
    `path#k` means part `k` of a mesh: the mesh is split into its connected components at load
    time (one `MeshInstance3D` per bulb of a string light), numbered as `bistro_lights.py` does.
    Dimming: a CoAP POST to `/l/dim` (`dim_uri`) received by the node with a payload 0..100 sets
    the level in percent (light energy and emission scaled; 0 is off). In the OTNS CLI:
    `send coap 2 7 "/l/dim" "56"` dims the luminaire of node 7 to 56 %, sent by node 2.
- `demo/`: a scene with an `OtnsClient`, an `OtnsField`, a free-flying camera and a status line.
- `tools/stream_test.gd`: headless check, prints the first events of a running OTNS and exits:

  ```
  godot4 --headless --path etc/godot-client -s tools/stream_test.gd -- --otns=127.0.0.1:8998 --events=20
  ```

## Bistro demo

`tools/install-addon.sh <bistro project dir>` copies the addon, `otns/bistro-lights.json` and
`otns/luminaires_test.gd` into the [Bistro Demo Tweaked](https://github.com/Jamsers/Bistro-Demo-Tweaked)
project. Its `MainScene.tscn` needs two nodes under the root (added once by hand or by the
editor): an `OtnsClient` (`host` 127.0.0.1, `port` 8998) and an `OtnsLuminaires` with
`mapping_file` `res://otns/bistro-lights.json` and `scene_root` `..`. Then:

```
otns -floorplan etc/floorplans/bistro.json      # 15 street lamps, 5 lanterns, 64 bulbs as nodes
godot4 --path <bistro project>                   # switch to a night scenario in the demo's UI
> radio 7 off                                    # in the OTNS CLI: the lamp of node 7 goes dark
> radio 7 on
> send coap 2 7 "/l/dim" "30"                    # node 2 dims node 7's lamp to 30 %
```

Headless check (needs the running OTNS):

```
godot4 --headless --path <bistro project> -s otns/luminaires_test.gd -- --otns=127.0.0.1:8998
```

## Notes

- gRPC-web over HTTP/1.1 supports server streaming, which is all the visualizer needs. Godot's
  `HTTPClient` has no HTTP/2, so the native gRPC port (base − 1) cannot be used directly.
- The stream resends the full state on connect, so the client can start or reconnect at any
  time.
- Decoding is pure GDScript, and at high simulation speeds the stream carries thousands of
  `Send` (frame sent) events per second. `OtnsClient` bounds the work per frame
  (`max_events_per_frame`, default 400), decodes only the last `AdvanceTime` of a batch, skips
  heartbeats, and drops `Send` events beyond `max_sends_per_frame` (default 20; the count is
  in `dropped_sends`). State events are never dropped. Without this, a large network at 100x
  made frames take seconds and the unary commands crawl.
- Coordinates: OTNS (x, y, z) with z up map to Godot (x, z, y) after the floor plan's scale and
  origin, the same mapping the web visualizer's 3D skin uses.
