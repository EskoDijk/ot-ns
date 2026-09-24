// Copyright (c) 2026, The OTNS Authors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the
//    names of its contributors may be used to endorse or promote products
//    derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.
//
// Spike for step 1 of studies/office-3d-skin-feasibility.md: three.js and Pixi 8 rendering into
// ONE canvas through a shared WebGL context.
//
//   - three.js draws the 3D "field": a floor with grid, 20 node spheres at (x, y, z) OTNS
//     positions, links between some nodes, and a drop line from each node to the floor.
//   - Pixi draws the HUD on top: a panel with text and a log, a draggable box, and one label per
//     node placed at the node's projected screen position (the plan for node labels in 3D).
//   - Pointer routing: every pointer event is handled by Pixi's stage first. A HUD object that
//     handles pointerdown stops propagation; otherwise the stage handler does three.js picking.
//     While a HUD or node drag is active, OrbitControls is disabled. This works because Pixi's DOM
//     listener is registered before OrbitControls' listener (Pixi is initialized first).
//   - '?autotest' in the page URL runs a scripted pointer sequence, samples pixels of the shared
//     drawing buffer, and writes PASS/FAIL lines to <pre id="results"> (and the console).
//
// Render order per frame, as documented by Pixi 8 for mixing with three.js:
//   three.resetState(); three.render(); pixi.resetState(); pixi.render();

import * as THREE from 'three';
import {OrbitControls} from 'three/addons/controls/OrbitControls.js';
import * as PIXI from 'pixi.js';

const NUM_NODES = 20;
const FIELD_SIZE = 600;    // OTNS units (1 unit = 1 px in the 2D visualizer)
const MAX_Z = 300;
const NODE_RADIUS = 20;    // same as the classic skin's circular shape radius
const COLOR_LEADER = 0xc62828;
const COLOR_ROUTER = 0x1565c0;
const COLOR_CHILD = 0x4caf50;
const COLOR_LINK = 0x1976d2;
const COLOR_DROP_LINE = 0x9e9e9e;
const COLOR_BACKGROUND = 0xf0f0f0;
const COLOR_HUD_PANEL = 0x263238;
const COLOR_HUD_BOX = 0xff8f00;
const LABEL_OFFSET = 11;

const autotest = new URLSearchParams(window.location.search).has('autotest');
const results = [];
let width = window.innerWidth;
let height = window.innerHeight;

// deterministic PRNG, so that the autotest sees the same layout every run.
function mulberry32(seed) {
    return function () {
        seed |= 0;
        seed = seed + 0x6D2B79F5 | 0;
        let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
        t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
        return ((t ^ t >>> 14) >>> 0) / 4294967296;
    };
}
const rand = mulberry32(20260924);

// OTNS coordinates: x right, y down (in the 2D view), z up. three.js: y up. A node "sits" on its
// z-height, so its sphere center is one radius above it.
function otnsToThree(x, y, z, target) {
    return target.set(x, z + NODE_RADIUS, y);
}

function threeToOtns(p) {
    return {x: Math.round(p.x), y: Math.round(p.z), z: Math.round(p.y - NODE_RADIUS)};
}

let hudLog = null;

function log(msg) {
    results.push(msg);
    console.log('[spike] ' + msg);
    if (hudLog !== null) {
        hudLog.text = results.slice(-10).join('\n');
    }
}

async function main() {
    // ---------------------------------------------------------------- three.js
    const three = new THREE.WebGLRenderer({antialias: true, stencil: true, preserveDrawingBuffer: autotest});
    three.setPixelRatio(1);
    three.setSize(width, height);
    three.setClearColor(COLOR_BACKGROUND, 1);
    document.body.appendChild(three.domElement);
    const canvas = three.domElement;

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(45, width / height, 1, 10000);
    camera.position.set(FIELD_SIZE / 2, FIELD_SIZE * 0.9, FIELD_SIZE * 1.7);

    scene.add(new THREE.HemisphereLight(0xffffff, 0x777777, 1.2));
    const sun = new THREE.DirectionalLight(0xffffff, 1.5);
    sun.position.set(1, 2, 1);
    scene.add(sun);

    const floor = new THREE.Mesh(new THREE.PlaneGeometry(FIELD_SIZE, FIELD_SIZE),
        new THREE.MeshLambertMaterial({color: 0xe3e3e3}));
    floor.rotation.x = -Math.PI / 2;
    floor.position.set(FIELD_SIZE / 2, 0, FIELD_SIZE / 2);
    scene.add(floor);
    const grid = new THREE.GridHelper(FIELD_SIZE, 12, 0x999999, 0xbdbdbd);
    grid.position.set(FIELD_SIZE / 2, 0.5, FIELD_SIZE / 2);
    scene.add(grid);

    const nodes = [];
    const sphereGeom = new THREE.SphereGeometry(NODE_RADIUS, 24, 16);
    for (let i = 0; i < NUM_NODES; i++) {
        const color = i === 0 ? COLOR_LEADER : (i < 8 ? COLOR_ROUTER : COLOR_CHILD);
        const mesh = new THREE.Mesh(sphereGeom, new THREE.MeshLambertMaterial({color: color}));
        const x = Math.round(60 + rand() * (FIELD_SIZE - 120));
        const y = Math.round(60 + rand() * (FIELD_SIZE - 120));
        const z = Math.round(rand() * MAX_Z / 100) * 100; // three floors: 0, 100, 200, 300
        otnsToThree(x, y, z, mesh.position);
        mesh.userData.nodeId = i + 1;
        scene.add(mesh);
        nodes.push(mesh);
    }

    // links: router i <-> router i+1, and each child to a router.
    const linkPairs = [];
    for (let i = 0; i < 7; i++) {
        linkPairs.push([i, i + 1]);
    }
    for (let i = 8; i < NUM_NODES; i++) {
        linkPairs.push([i, i % 8]);
    }
    const linkGeom = new THREE.BufferGeometry();
    linkGeom.setAttribute('position', new THREE.BufferAttribute(new Float32Array(linkPairs.length * 6), 3));
    const links = new THREE.LineSegments(linkGeom, new THREE.LineBasicMaterial({color: COLOR_LINK}));
    scene.add(links);
    const dropGeom = new THREE.BufferGeometry();
    dropGeom.setAttribute('position', new THREE.BufferAttribute(new Float32Array(NUM_NODES * 6), 3));
    const dropLines = new THREE.LineSegments(dropGeom, new THREE.LineBasicMaterial({color: COLOR_DROP_LINE}));
    scene.add(dropLines);

    function updateLines() {
        const lp = linkGeom.attributes.position.array;
        linkPairs.forEach(([a, b], i) => {
            nodes[a].position.toArray(lp, i * 6);
            nodes[b].position.toArray(lp, i * 6 + 3);
        });
        linkGeom.attributes.position.needsUpdate = true;
        const dp = dropGeom.attributes.position.array;
        nodes.forEach((n, i) => {
            n.position.toArray(dp, i * 6);
            dp[i * 6 + 3] = n.position.x;
            dp[i * 6 + 4] = 0;
            dp[i * 6 + 5] = n.position.z;
        });
        dropGeom.attributes.position.needsUpdate = true;
    }
    updateLines();

    // ---------------------------------------------------------------- Pixi, on the same context
    const pixi = new PIXI.WebGLRenderer();
    await pixi.init({
        context: three.getContext(),
        canvas: canvas,
        width: width,
        height: height,
        resolution: 1,
        clearBeforeRender: false, // keep the three.js output
        backgroundAlpha: 0,
        antialias: true,
    });
    const stage = new PIXI.Container();
    stage.eventMode = 'static';
    stage.hitArea = new PIXI.Rectangle(0, 0, width, height);

    const labelLayer = new PIXI.Container();
    stage.addChild(labelLayer);
    const labels = nodes.map((n) => {
        const rloc16 = (0x0400 * n.userData.nodeId).toString(16).toUpperCase().padStart(4, '0');
        const t = new PIXI.Text({text: n.userData.nodeId + '|' + rloc16, style: {fontFamily: 'Arial', fontSize: 12, fill: 0x000000}});
        labelLayer.addChild(t);
        return t;
    });

    const hud = new PIXI.Container();
    stage.addChild(hud);
    const panel = new PIXI.Graphics().roundRect(0, 0, 330, 210, 8).fill({color: COLOR_HUD_PANEL, alpha: 0.9});
    panel.position.set(10, 10);
    hud.addChild(panel);
    const title = new PIXI.Text({text: 'OTNS spike: Pixi 8 HUD over three.js, one canvas', style: {fontFamily: 'Arial', fontSize: 14, fill: 0xffffff, fontWeight: 'bold'}});
    title.position.set(20, 18);
    hud.addChild(title);
    const fpsText = new PIXI.Text({text: '', style: {fontFamily: 'Arial', fontSize: 12, fill: 0xb0bec5}});
    fpsText.position.set(20, 40);
    hud.addChild(fpsText);
    hudLog = new PIXI.Text({text: '', style: {fontFamily: 'monospace', fontSize: 11, fill: 0xffffff, lineHeight: 14}});
    hudLog.position.set(20, 60);
    hud.addChild(hudLog);

    const box = new PIXI.Container();
    box.addChild(new PIXI.Graphics().roundRect(0, 0, 70, 40, 6).fill(COLOR_HUD_BOX));
    const boxLabel = new PIXI.Text({text: 'drag me', style: {fontFamily: 'Arial', fontSize: 12, fill: 0x000000}});
    boxLabel.position.set(10, 12);
    box.addChild(boxLabel);
    box.position.set(width - 100, 20);
    box.eventMode = 'static';
    box.cursor = 'grab';
    hud.addChild(box);

    // ---------------------------------------------------------------- camera control, after Pixi
    const controls = new OrbitControls(camera, canvas);
    controls.target.set(FIELD_SIZE / 2, 0, FIELD_SIZE / 2);
    controls.update();

    // ---------------------------------------------------------------- pointer routing
    const raycaster = new THREE.Raycaster();
    const ndc = new THREE.Vector2();
    const dragPlane = new THREE.Plane();
    const hitPoint = new THREE.Vector3();
    const dragOffset = new THREE.Vector3();
    let dragNode = null;
    let hudDrag = null;

    function setRay(sx, sy) {
        ndc.set(sx / width * 2 - 1, -(sy / height * 2 - 1));
        raycaster.setFromCamera(ndc, camera);
    }

    function pickNode(sx, sy) {
        setRay(sx, sy);
        const hits = raycaster.intersectObjects(nodes, false);
        return hits.length > 0 ? hits[0] : null;
    }

    box.on('pointerdown', (e) => {
        hudDrag = {dx: e.global.x - box.x, dy: e.global.y - box.y};
        controls.enabled = false;
        log('HUD box drag start, orbit disabled');
        e.stopPropagation();
    });

    stage.on('pointerdown', (e) => {
        const hit = pickNode(e.global.x, e.global.y);
        if (hit === null) {
            log('pointerdown on empty space: orbit');
            return;
        }
        dragNode = hit.object;
        controls.enabled = false;
        // drag in the node's own horizontal plane (constant z)
        dragPlane.set(new THREE.Vector3(0, 1, 0), -dragNode.position.y);
        raycaster.ray.intersectPlane(dragPlane, hitPoint);
        dragOffset.subVectors(hitPoint, dragNode.position);
        const p = threeToOtns(dragNode.position);
        log(`picked node ${dragNode.userData.nodeId} at (${p.x},${p.y},${p.z})`);
    });

    const lastPointer = {x: 0, y: 0, moves: 0};
    stage.on('globalpointermove', (e) => {
        lastPointer.x = e.global.x;
        lastPointer.y = e.global.y;
        lastPointer.moves++;
        if (hudDrag !== null) {
            box.position.set(e.global.x - hudDrag.dx, e.global.y - hudDrag.dy);
        } else if (dragNode !== null) {
            setRay(e.global.x, e.global.y);
            if (raycaster.ray.intersectPlane(dragPlane, hitPoint) !== null) {
                dragNode.position.x = hitPoint.x - dragOffset.x;
                dragNode.position.z = hitPoint.z - dragOffset.z;
                updateLines();
            }
        }
    });

    function endDrag() {
        if (hudDrag !== null) {
            hudDrag = null;
            controls.enabled = true;
            log('HUD box drag end, orbit enabled');
        }
        if (dragNode !== null) {
            const p = threeToOtns(dragNode.position);
            log(`dropped node ${dragNode.userData.nodeId} at (${p.x},${p.y},${p.z}) -> 'move ${dragNode.userData.nodeId} ${p.x} ${p.y} ${p.z}'`);
            dragNode = null;
            controls.enabled = true;
        }
    }
    stage.on('pointerup', endDrag);
    stage.on('pointerupoutside', endDrag);

    // ---------------------------------------------------------------- labels at projected positions
    const projected = new THREE.Vector3();

    function projectToScreen(p) {
        projected.copy(p).project(camera);
        return {x: (projected.x + 1) / 2 * width, y: (1 - projected.y) / 2 * height, visible: projected.z < 1};
    }

    function updateLabels() {
        nodes.forEach((n, i) => {
            const s = projectToScreen(n.position);
            labels[i].position.set(s.x + LABEL_OFFSET, s.y + LABEL_OFFSET);
            labels[i].visible = s.visible;
        });
    }

    // ---------------------------------------------------------------- frame loop
    const gl = three.getContext();
    const pixelRequests = []; // {x, y, resolve}, sampled after both renderers drew this frame
    let frames = 0;
    let lastFpsTime = performance.now();

    function frame() {
        controls.update();
        three.resetState();
        three.render(scene, camera);
        pixi.resetState();
        updateLabels();
        pixi.render({container: stage});
        while (pixelRequests.length > 0) {
            const r = pixelRequests.shift();
            const px = new Uint8Array(4);
            gl.readPixels(Math.round(r.x), Math.round(height - r.y), 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, px);
            r.resolve([px[0], px[1], px[2]]);
        }
        frames++;
        const now = performance.now();
        if (now - lastFpsTime >= 1000) {
            fpsText.text = `${frames} fps, ${nodes.length} nodes, ${linkPairs.length} links; drag a sphere or the box, orbit elsewhere`;
            frames = 0;
            lastFpsTime = now;
        }
        requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);

    window.addEventListener('resize', () => {
        width = window.innerWidth;
        height = window.innerHeight;
        three.setSize(width, height);
        camera.aspect = width / height;
        camera.updateProjectionMatrix();
        pixi.resize(width, height);
        stage.hitArea = new PIXI.Rectangle(0, 0, width, height);
    });

    log(`three.js r${THREE.REVISION}, pixi.js ${PIXI.VERSION}, ${width}x${height}`);

    // ---------------------------------------------------------------- autotest
    const spike = {three, pixi, scene, camera, controls, nodes, box, canvas, pickNode, projectToScreen,
        samplePixel: (x, y) => new Promise((resolve) => pixelRequests.push({x, y, resolve})),
        // resolves after the next rendered frame: frame() re-registers itself before any test code runs.
        nextFrame: () => new Promise((resolve) => requestAnimationFrame(resolve)),
        getDragNode: () => dragNode, lastPointer, results};
    window.spike = spike;
    if (autotest) {
        await runAutotest(spike);
    }
}

async function runAutotest(s) {
    const lines = [];
    const resultsEl = document.getElementById('results');
    const check = (ok, what) => {
        lines.push((ok ? 'PASS ' : 'FAIL ') + what);
        log((ok ? 'PASS ' : 'FAIL ') + what);
        resultsEl.textContent = lines.join('\n'); // progressive, so that a headless DOM dump shows partial results
    };
    // synthetic PointerEvents have no active pointer, so pointer capture (used by OrbitControls) must be a no-op.
    s.canvas.setPointerCapture = () => {};
    s.canvas.releasePointerCapture = () => {};
    const pointer = (type, x, y) => s.canvas.dispatchEvent(new PointerEvent(type, {
        clientX: x, clientY: y, bubbles: true, cancelable: true, pointerId: 1, pointerType: 'mouse',
        isPrimary: true, button: 0, buttons: type === 'pointerup' ? 0 : 1,
    }));
    const isBackground = (px) => Math.abs(px[0] - 0xf0) < 6 && Math.abs(px[1] - 0xf0) < 6 && Math.abs(px[2] - 0xf0) < 6;

    // headless Chrome may resize the window shortly after load; wait until the viewport is stable.
    for (let stable = 0, w = 0, h = 0, i = 0; stable < 3 && i < 12; i++) {
        await s.nextFrame();
        stable = (window.innerWidth === w && window.innerHeight === h) ? stable + 1 : 0;
        w = window.innerWidth;
        h = window.innerHeight;
    }
    log(`autotest starts at ${window.innerWidth}x${window.innerHeight}`);
    check(document.querySelectorAll('canvas').length === 1, 'exactly one canvas in the document');
    check(s.pixi.canvas === s.canvas && s.pixi.gl === s.three.getContext(), 'Pixi uses the three.js canvas and GL context');

    // 1. both engines draw into the same buffer: a sphere pixel and a HUD panel pixel
    const leader = s.nodes[0];
    const lp = s.projectToScreen(leader.position);
    const spherePx = await s.samplePixel(lp.x, lp.y);
    check(spherePx[0] > spherePx[1] + 40 && spherePx[0] > spherePx[2] + 40, `leader sphere pixel is red-ish: ${spherePx}`);
    const panelPx = await s.samplePixel(30, 100);
    check(panelPx[0] < 90 && panelPx[1] < 100 && panelPx[2] < 110 && !isBackground(panelPx), `HUD panel pixel is dark: ${panelPx}`);
    // a point on the floor, in front of the field center: projected, so independent of the viewport size
    const floorPt = s.projectToScreen(new THREE.Vector3(FIELD_SIZE / 2, 0, FIELD_SIZE * 0.95));
    const floorPx = await s.samplePixel(floorPt.x, floorPt.y);
    check(!isBackground(floorPx), `floor pixel is drawn at (${floorPt.x.toFixed(0)},${floorPt.y.toFixed(0)}): ${floorPx}`);

    // 2. HUD drag: Pixi gets the event, OrbitControls stays out
    const camBefore = s.camera.position.clone();
    const bx = s.box.x + 35, by = s.box.y + 20;
    pointer('pointerdown', bx, by);
    check(s.controls.enabled === false, 'orbit disabled on HUD pointerdown (Pixi listener runs before OrbitControls)');
    pointer('pointermove', bx + 40, by + 30);
    await s.nextFrame();
    check(Math.round(s.box.x) === Math.round(bx - 35 + 40) && Math.round(s.box.y) === Math.round(by - 20 + 30), `HUD box followed the pointer to (${s.box.x},${s.box.y})`);
    check(s.camera.position.distanceTo(camBefore) < 1e-6, 'camera did not move during HUD drag');
    pointer('pointerup', bx + 40, by + 30);
    check(s.controls.enabled === true, 'orbit re-enabled after HUD drag');

    // 3. node drag on its horizontal plane
    const p0 = leader.position.clone();
    const np = s.projectToScreen(leader.position);
    pointer('pointerdown', np.x, np.y);
    check(s.getDragNode() === leader, 'raycast picked the leader sphere under the pointer');
    check(s.controls.enabled === false, 'orbit disabled during node drag');
    pointer('pointermove', np.x + 60, np.y);
    await s.nextFrame();
    check(Math.abs(leader.position.y - p0.y) < 1e-6 && Math.abs(leader.position.x - p0.x) > 20, `node moved in its plane: ${p0.x.toFixed(0)} -> ${leader.position.x.toFixed(0)}, height unchanged`);
    const lp2 = s.projectToScreen(leader.position);
    const rect = s.canvas.getBoundingClientRect();
    check(s.pickNode(np.x + 60, np.y) !== null && s.pickNode(np.x + 60, np.y).object === leader,
        `node stays under the pointer: center now (${lp2.x.toFixed(1)},${lp2.y.toFixed(1)}), pointer (${(np.x + 60).toFixed(1)},${np.y.toFixed(1)}), ` +
        `Pixi saw (${s.lastPointer.x.toFixed(1)},${s.lastPointer.y.toFixed(1)}) after ${s.lastPointer.moves} moves, canvas rect ${rect.width}x${rect.height}@${rect.left},${rect.top}`);
    pointer('pointerup', np.x + 60, np.y);
    check(s.getDragNode() === null && s.controls.enabled === true, 'node drag ended, orbit re-enabled');

    // 4. drag on empty space orbits the camera
    let ex = floorPt.x, ey = floorPt.y;
    for (let tries = 0; s.pickNode(ex, ey) !== null && tries < 5; tries++) {
        ex += 80;
    }
    const camBefore2 = s.camera.position.clone();
    pointer('pointerdown', ex, ey);
    check(s.controls.enabled === true, 'orbit stays enabled on empty-space pointerdown');
    pointer('pointermove', ex + 50, ey);
    await s.nextFrame();
    check(s.camera.position.distanceTo(camBefore2) > 1, `camera orbited: moved ${s.camera.position.distanceTo(camBefore2).toFixed(1)} units`);
    pointer('pointerup', ex + 50, ey);
    await s.nextFrame();

    const failed = lines.filter((l) => l.startsWith('FAIL')).length;
    lines.push(`${lines.length - failed}/${lines.length} checks passed`);
    resultsEl.textContent = lines.join('\n');
    window.spikeDone = true;
}

main().catch((err) => {
    console.error(err);
    const el = document.getElementById('results');
    el.textContent = 'FAIL: ' + (err.stack || err);
    window.spikeDone = true;
});
