# Web visualizer spikes

Throw-away experiments for the web visualizer. Nothing here is embedded into the `otns`
binary (`script/pack-web` packs only `templates/` and `static/`), and the pages have no gRPC
connection. Delete a spike once the feature it explored has landed.

## e2e-otns: the real visualizer page against a live simulation

`e2e-otns.sh <variant>...` starts `otns` (`OTNS_BIN`), `grpcwebproxy` and a static server for
page variants in `$E2E_DIR/site` (default `/tmp/otns-e2e`), runs the CLI commands of
`$E2E_DIR/cmds.txt`, and screenshots each `visualize-<variant>.html` with `headless-run.mjs` once
`done.js` (an expression polled in the page, e.g. "all nodes attached, then select node 2") is
true, after evaluating `action.js` (e.g. a synthetic pointer drag of a node). The page bundle
exposes `window.otnsVis` for this. Used to verify the field-renderer refactor (step 2 of the 3D
study) against the committed bundle: same drawing, and a drag of node 1 produced the expected
`move` commands.

## pixi-three: Pixi 8 and three.js on one canvas

Step 1 of `studies/office-3d-skin-feasibility.md`: verify that a three.js 3D scene and the
existing Pixi HUD can share one canvas and one WebGL context, that pointer events can be routed
through Pixi first, and that node labels can be Pixi text placed at projected 3D positions.

Build (from `web/site`, after `npm install` has installed `three`):

```sh
npx webpack --config spike/webpack.config.js
```

Open `spike/pixi-three.html` in a browser (a `file://` URL works). Drag a sphere: it moves in
its own horizontal plane and the log shows the `move` command that OTNS would receive. Drag the
orange box: a Pixi HUD drag, during which the camera stays put. Drag anywhere else to orbit,
wheel to zoom.

Automated check in headless Chrome (software GL via SwiftShader, so expect a few fps):

```sh
node spike/headless-run.mjs "file://$PWD/spike/pixi-three.html?autotest" shot.png
```

`headless-run.mjs` drives Chrome over the DevTools protocol (no npm dependencies, Node >= 22):
it waits until the page sets `window.spikeDone`, saves a screenshot, prints the console output
and the `<pre id="results">` text, and exits non-zero on a `FAIL` line or a timeout. Chrome's own
`--screenshot --dump-dom` mode is not usable here: it dumps after a fixed few seconds regardless
of `--virtual-time-budget`, which is not enough for a software-rendered WebGL page.

The `?autotest` page dispatches synthetic pointer events, samples pixels of the shared drawing
buffer and prints `PASS`/`FAIL` lines to the console and into `<pre id="results">`.

Findings (2026-09-24, three.js r186, pixi.js 8.18.1, Chrome 152):

- One canvas, one context: `PIXI.WebGLRenderer.init({context, canvas, clearBeforeRender: false})`
  and per frame `three.resetState(); three.render(); pixi.resetState(); pixi.render()`. Both
  engines' output is present in the same drawing buffer (pixel samples of a sphere, the floor and
  the HUD panel).
- Pointer routing: Pixi's stage receives every pointer event; a HUD object stops propagation,
  otherwise the stage handler raycasts into the three.js scene. Because Pixi is initialized before
  `OrbitControls`, its DOM listener runs first and can set `controls.enabled = false` in the same
  event, so a HUD or node drag never orbits the camera.
- Node drag in the node's horizontal plane keeps the node under the pointer and its height fixed.
- Headless Chrome resizes its window shortly after load (713 to 800 px high here); a resize during a
  drag changes the projection, so the autotest waits for a stable viewport first.
- Bundle: Pixi + three.js core + OrbitControls, minified: 1010 KiB (269 KB gzip). three.js core +
  OrbitControls alone: 753 KiB (189 KB gzip).
