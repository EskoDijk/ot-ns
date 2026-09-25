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
// ThreeFieldRenderer draws the field in 3D with three.js, into the same canvas and WebGL context
// as Pixi: each frame it renders its scene first, then Pixi draws the HUD and the node labels on
// top (Pixi's clearBeforeRender is off while this renderer is active). See FieldRenderer.js for
// the interface, and skins/office3d.js for the skin that selects this renderer.
//
// Coordinates: OTNS (x, y, z) with z the height maps to three.js (x, z, y), so that the 2D view's
// "down the screen" (+y) is the depth axis and three.js's y axis is up. A top view (key 't') then
// shows the same picture as the 2D skins.
//
// A building (floor plan) is fetched from /floorplan.json of the site server, which serves the
// file given with 'otns -floorplan'; see etc/floorplans/README.md. Its nodeScale shrinks the node
// shapes (and marks and messages) to the building's scale; node positions are not affected.
//
// Pointer handling: Pixi delivers pointer events to the visualizer's root container when no HUD
// element handles them; this renderer listens there, raycasts into the scene to select and drag
// nodes, and lets OrbitControls rotate/pan only for a press on empty space. A node is dragged in
// its horizontal plane (x, y change); with the Alt key held it is dragged vertically instead
// (only z changes). Alt can be pressed or released during a drag.

import * as THREE from 'three';
import {OrbitControls} from 'three/addons/controls/OrbitControls.js';
import * as PIXI from "pixi.js";
import FieldRenderer from "../FieldRenderer";
import Building from "./Building";
import Lifetime from "../Lifetime";
import {Resources} from "../resources";
import {Skin} from "../skins";
import {NODE_LABEL_FONT_FAMILY, NODE_LABEL_FONT_SIZE} from "../consts";

const GRID_SIZE = 2000;        // OTNS units covered by the floor grid, from (0,0)
const GRID_STEP = 100;
const DRAG_START_DISTANCE = 5; // px of pointer movement before a press becomes a drag
const DRAG_MOVE_INTERVAL = 0.2; // s between 'move' commands while dragging
const PARTITION_BAND_TUBE = 3;
const FAILED_MARK_SIZE = 24;
const ACK_RISE = 50;           // units an ACK message rises
const UNKNOWN_DST_DISTANCE = 200; // units a unicast to an unknown destination travels
const COLOR_GRID_CENTER = 0x9e9e9e;
const COLOR_GRID = 0xdddddd;
const COLOR_DROP_LINE = 0x9e9e9e;
const DROP_LINE_DASH = 3;      // units; dash and gap length of the dotted height line
const UP = new THREE.Vector3(0, 1, 0);
const FLOOR_PLAN_URL = '/floorplan.json';

function toThree(x, y, z, target) {
    return target.set(x, z, y);
}

/**
 * The floor grid, from OTNS (0,0) to (GRID_SIZE, GRID_SIZE), with the lines through the origin in
 * a darker color. It is built from one short segment per cell edge rather than one long line per
 * row/column: some GL implementations (e.g. SwiftShader) drop a whole line of which one endpoint
 * is behind the camera, which would blank out the grid lines running away from the viewer.
 */
function makeGrid() {
    const n = GRID_SIZE / GRID_STEP;
    const points = [];
    const colors = [];
    const dark = new THREE.Color(COLOR_GRID_CENTER);
    const light = new THREE.Color(COLOR_GRID);
    const addSegment = (x1, z1, x2, z2, color) => {
        points.push(x1, 0, z1, x2, 0, z2);
        colors.push(color.r, color.g, color.b, color.r, color.g, color.b);
    };
    for (let i = 0; i <= n; i++) {
        const k = i * GRID_STEP;
        const color = i === 0 ? dark : light;
        for (let j = 0; j < n; j++) {
            addSegment(k, j * GRID_STEP, k, (j + 1) * GRID_STEP, color);         // along the depth axis
            addSegment(j * GRID_STEP, k, (j + 1) * GRID_STEP, k, color);         // along x
        }
    }
    const geom = new THREE.BufferGeometry();
    geom.setAttribute('position', new THREE.Float32BufferAttribute(points, 3));
    geom.setAttribute('color', new THREE.Float32BufferAttribute(colors, 3));
    return new THREE.LineSegments(geom, new THREE.LineBasicMaterial({vertexColors: true, toneMapped: false}));
}

function fromThree(p) {
    return {x: Math.round(p.x), y: Math.round(p.z), z: Math.round(p.y)};
}

// The 3D drawing of one node: its body mesh, partition band, drop line to the floor, 'failed'
// mark, and a Pixi label positioned at the projected node position.
class ThreeNodeView {
    constructor(renderer, state) {
        this.renderer = renderer;
        this.state = state;
        this.dragging = false;
        this.group = new THREE.Group();
        this.group.userData.nodeId = state.id;
        this.body = null;
        this.partitionBand = null;
        this.selection = null;

        // dotted line from the node down to the floor, showing its height
        this.dropLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]),
            new THREE.LineDashedMaterial({color: COLOR_DROP_LINE, dashSize: DROP_LINE_DASH, gapSize: DROP_LINE_DASH}));
        this.group.add(this.dropLine);

        this.failedMark = new THREE.Sprite(new THREE.SpriteMaterial({map: renderer.failedMarkTexture(), depthTest: false}));
        this.failedMark.scale.set(FAILED_MARK_SIZE, FAILED_MARK_SIZE, 1);
        this.failedMark.renderOrder = 2;
        this.group.add(this.failedMark);

        this.label = new PIXI.Text({text: "", style: {fontFamily: NODE_LABEL_FONT_FAMILY, fontSize: NODE_LABEL_FONT_SIZE, align: 'left'}});

        this.onPositionChanged();
        this.redraw();
    }

    get id() {
        return this.state.id;
    }

    redraw() {
        const skin = Skin();
        const style = skin.node3DStyle(this.state);
        const radius = style.radius * this.renderer.nodeScale;
        if (this.body !== null) {
            this.group.remove(this.body);
            this.body.material.dispose();
        }
        this.body = new THREE.Mesh(this.renderer.bodyGeometry(style.shape, radius), new THREE.MeshLambertMaterial({
            color: style.color, transparent: style.opacity < 1, opacity: style.opacity, wireframe: style.wireframe,
        }));
        this.body.userData.nodeId = this.state.id;
        this.group.add(this.body);
        this.radius = radius;
        const markSize = FAILED_MARK_SIZE * this.renderer.nodeScale;
        this.failedMark.scale.set(markSize, markSize, 1);

        if (this.partitionBand !== null) {
            this.group.remove(this.partitionBand);
            this.partitionBand.material.dispose();
        }
        this.partitionBand = new THREE.Mesh(this.renderer.bandGeometry(radius),
            new THREE.MeshLambertMaterial({color: this.renderer.vis.getPartitionColor(this.state.partition)}));
        this.partitionBand.rotation.x = Math.PI / 2;
        this.partitionBand.visible = this.renderer.partitionVisible;
        this.group.add(this.partitionBand);

        this.failedMark.visible = this.state.failed;
        const rloc16 = ('0000' + this.state.rloc16.toString(16).toUpperCase()).slice(-4);
        this.label.text = this.state.id.toString() + "|" + rloc16;

        if (this.selection !== null) {
            this.setSelected(false);
            this.setSelected(true);
        }
    }

    setPartitionVisible(visible) {
        this.partitionBand.visible = visible;
    }

    /**
     * The state's position changed. While the user drags the node, the drag position is kept.
     */
    onPositionChanged() {
        if (!this.dragging) {
            toThree(this.state.x, this.state.y, this.state.z, this.group.position);
        }
        this._updateDropLine();
    }

    _updateDropLine() {
        const pos = this.dropLine.geometry.attributes.position;
        pos.setXYZ(0, 0, 0, 0);
        pos.setXYZ(1, 0, -this.group.position.y, 0);
        pos.needsUpdate = true;
        this.dropLine.computeLineDistances(); // the dash pattern needs the line length
    }

    setSelected(selected) {
        if (selected === (this.selection !== null)) {
            return;
        }
        if (!selected) {
            this.group.remove(this.selection);
            this.selection.traverse((obj) => {
                if (obj.material) obj.material.dispose();
                if (obj.geometry) obj.geometry.dispose();
            });
            this.selection = null;
            return;
        }
        const style = Skin().selectionStyle();
        const sel = new THREE.Group();
        // dashed box around the node
        const boxSize = style.boxSize * this.renderer.nodeScale;
        const box = new THREE.LineSegments(new THREE.EdgesGeometry(new THREE.BoxGeometry(boxSize, boxSize, boxSize)),
            new THREE.LineDashedMaterial({color: style.boxColor, dashSize: 6, gapSize: 4, transparent: true, opacity: style.boxAlpha}));
        box.computeLineDistances();
        sel.add(box);
        // the radio range: a translucent sphere (lighter than the 2D disc, since it fills much of
        // the view when seen from nearby), and its outline in the node's horizontal plane
        const range = this.state.radioRange;
        sel.add(new THREE.Mesh(new THREE.SphereGeometry(range, 48, 24), new THREE.MeshBasicMaterial({
            color: style.rangeFill, transparent: true, opacity: style.rangeFillAlpha * 0.5, depthWrite: false, side: THREE.DoubleSide,
        })));
        const ring = new THREE.LineLoop(new THREE.BufferGeometry().setFromPoints(
            new THREE.EllipseCurve(0, 0, range, range).getPoints(96).map((p) => new THREE.Vector3(p.x, 0, p.y))),
            new THREE.LineBasicMaterial({color: style.rangeStroke, transparent: true, opacity: style.rangeStrokeAlpha}));
        sel.add(ring);
        this.selection = sel;
        this.group.add(sel);
    }

    destroy() {
        if (this.selection !== null) {
            this.setSelected(false);
        }
        this.body.material.dispose();
        this.partitionBand.material.dispose();
        this.dropLine.geometry.dispose();
        this.dropLine.material.dispose();
        this.failedMark.material.dispose();
        this.label.destroy();
    }
}

// An animated message: a mesh in the scene with a Lifetime; update() returns false when over.
class ThreeMessageView {
    constructor(renderer, mvInfo, mesh) {
        this.renderer = renderer;
        this.mesh = mesh;
        this.lifetime = new Lifetime(renderer.vis);
        this.lifetime.configure(mvInfo);
    }

    update(dt) {
        this.lifetime.update(dt);
        return !this.lifetime.isOver();
    }

    destroy() {
        this.mesh.material.dispose();
    }
}

class BroadcastMessage extends ThreeMessageView {
    constructor(renderer, src, mvInfo) {
        const style = Skin().messageStyle('broadcast');
        const mesh = new THREE.Mesh(renderer.ringGeometry(), new THREE.MeshBasicMaterial({
            color: style.color, transparent: true, opacity: 0.3, depthWrite: false, side: THREE.DoubleSide,
        }));
        mesh.rotation.x = -Math.PI / 2;
        super(renderer, mvInfo, mesh);
        this.src = src;
        this.beginRadius = style.size * renderer.nodeScale;
        this.targetRadius = src.state.radioRange;
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        const radius = this.beginRadius + (this.targetRadius - this.beginRadius) * Math.pow(this.lifetime.progress(), 0.5);
        this.mesh.scale.set(radius, radius, 1);
        this.mesh.position.copy(this.src.group.position);
        return true;
    }
}

class UnicastMessage extends ThreeMessageView {
    constructor(renderer, src, dst, mvInfo) {
        const style = Skin().messageStyle('unicast');
        super(renderer, mvInfo, new THREE.Mesh(renderer.bodyGeometry('hexprism', style.size / 2 * renderer.nodeScale),
            new THREE.MeshLambertMaterial({color: style.color})));
        this.src = src;
        this.dst = dst;
        this.mesh.position.copy(src.group.position);
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        const from = this.src.group.position;
        const to = this.dst !== null ? this.dst.group.position : from.clone().add(new THREE.Vector3(0, 0, UNKNOWN_DST_DISTANCE));
        this.mesh.position.lerpVectors(from, to, this.lifetime.progress());
        return true;
    }
}

class AckMessage extends ThreeMessageView {
    constructor(renderer, src, mvInfo) {
        const style = Skin().messageStyle('ack');
        super(renderer, mvInfo, new THREE.Mesh(new THREE.TetrahedronGeometry((style.size / 2 + 2) * renderer.nodeScale),
            new THREE.MeshLambertMaterial({color: style.color})));
        this.src = src;
        this.mesh.position.copy(src.group.position);
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        this.mesh.position.copy(this.src.group.position);
        this.mesh.position.y += ACK_RISE * this.renderer.nodeScale * this.lifetime.progress();
        return true;
    }

    destroy() {
        super.destroy();
        this.mesh.geometry.dispose();
    }
}

export default class ThreeFieldRenderer extends FieldRenderer {
    constructor(vis) {
        super(vis);
        this.kind = 'three';
        this.partitionVisible = vis.visOptions.partitionId;
        this._views = {};      // node ID -> ThreeNodeView
        this._messages = [];
        this._geometries = {}; // cache of shared geometries, see bodyGeometry()
        this._selected = null;
        this._framed = false;
        this._destroyed = false;
        this.building = null;  // Building, once the floor plan is loaded
        this.nodeScale = 1.0;  // scale of node shapes, from the floor plan

        const pixi = vis.app.renderer;
        this._pixi = pixi;
        this._three = new THREE.WebGLRenderer({canvas: pixi.canvas, context: pixi.gl});
        this._three.setPixelRatio(pixi.resolution);
        this._three.setSize(pixi.screen.width, pixi.screen.height, false);
        this._three.setClearColor(0x000000, 0); // transparent: the page background shows, as with Pixi
        pixi.background.clearBeforeRender = false;

        this._scene = new THREE.Scene();
        this._camera = new THREE.PerspectiveCamera(45, pixi.screen.width / pixi.screen.height, 1, 20000);
        this._scene.add(new THREE.HemisphereLight(0xffffff, 0x888888, 1.3));
        const sun = new THREE.DirectionalLight(0xffffff, 1.2);
        sun.position.set(1, 2, 1);
        this._scene.add(sun);
        this._scene.add(makeGrid());

        this._nodesGroup = new THREE.Group();
        this._scene.add(this._nodesGroup);
        this._linksGroup = new THREE.Group();
        this._scene.add(this._linksGroup);
        this._messagesGroup = new THREE.Group();
        this._scene.add(this._messagesGroup);

        this._backLayer = new PIXI.Container(); // nothing: the 3D scene is behind everything Pixi draws
        this._frontLayer = new PIXI.Container(); // the node labels
        this._labelsLayer = this._frontLayer;

        // Pixi's pointer listeners were registered before OrbitControls', so the handlers below run
        // first for the same DOM event and can enable/disable the camera controls per press.
        this._controls = new OrbitControls(this._camera, pixi.canvas);
        this._controls.enableRotate = false;
        this._controls.enablePan = false;
        this._raycaster = new THREE.Raycaster();
        this._drag = null;
        this._press = null;
        this._onPointerDown = (e) => this._pointerDown(e);
        this._onPointerMove = (e) => this._pointerMove(e);
        this._onPointerUp = (e) => this._pointerUp(e);
        const root = vis.root;
        root.on('pointerdown', this._onPointerDown);
        root.on('globalpointermove', this._onPointerMove);
        root.on('pointerup', this._onPointerUp);
        root.on('pointerupoutside', this._onPointerUp);

        this.resetView();
        this._framed = false; // frame the nodes once they are added, see update()
        this._loadBuilding();
    }

    /**
     * Fetch the floor plan from the site server, if it serves one, and add the building.
     */
    _loadBuilding() {
        if (typeof fetch !== 'function' || !window.location.protocol.startsWith('http')) {
            return;
        }
        fetch(FLOOR_PLAN_URL, {cache: 'no-cache'}).then((response) => {
            if (!response.ok) {
                return null; // 404: no floor plan configured
            }
            return response.json();
        }).then((plan) => {
            if (plan === null || this._destroyed) {
                return;
            }
            this.setBuilding(new Building(plan));
            this.vis.log(`Floor plan loaded: ${this.building.name} (${this.building.floors.length} floors)`);
        }).catch((err) => {
            console.error("floor plan: " + err);
            this.vis.log("Floor plan could not be loaded, see the console");
        });
    }

    /**
     * Nodes on a hidden floor of the building are hidden too (their links as well, see drawLinks).
     */
    _applyFloorVisibility() {
        for (let id in this._views) {
            const view = this._views[id];
            view.group.visible = this.building === null || this.building.isHeightVisible(view.group.position.y);
        }
    }

    /**
     * Show `building` (a Building, or null for none), scaling the node shapes to it.
     */
    setBuilding(building) {
        if (this.building !== null) {
            this._scene.remove(this.building.group);
            this.building.dispose();
        }
        this.building = building;
        this.nodeScale = building !== null ? building.nodeScale : 1.0;
        if (building !== null) {
            this._scene.add(building.group);
        }
        this.applySkin(); // redraw the nodes at the new scale
        this._framed = false; // re-frame including the building, see update()
    }

    get backLayer() {
        return this._backLayer;
    }

    get frontLayer() {
        return this._frontLayer;
    }

    // ---------------------------------------------------------------- shared geometries

    /**
     * @returns a cached geometry of the node body shape, see Office3DSkin.node3DStyle()
     */
    bodyGeometry(shape, radius) {
        const key = shape + ":" + radius;
        if (!(key in this._geometries)) {
            let geom;
            switch (shape) {
                case 'cube':
                    geom = new THREE.BoxGeometry(radius * 2, radius * 2, radius * 2);
                    break;
                case 'hexprism':
                    geom = new THREE.CylinderGeometry(radius, radius, radius * 1.2, 6);
                    break;
                default:
                    geom = new THREE.SphereGeometry(radius, 24, 16);
                    break;
            }
            this._geometries[key] = geom;
        }
        return this._geometries[key];
    }

    bandGeometry(radius) {
        const key = "band:" + radius;
        if (!(key in this._geometries)) {
            this._geometries[key] = new THREE.TorusGeometry(radius * 1.05, PARTITION_BAND_TUBE * this.nodeScale, 8, 32);
        }
        return this._geometries[key];
    }

    ringGeometry() {
        if (!("ring" in this._geometries)) {
            this._geometries["ring"] = new THREE.RingGeometry(0.97, 1.0, 64);
        }
        return this._geometries["ring"];
    }

    failedMarkTexture() {
        if (!this._failedMarkTexture) {
            // reuse the image that Pixi loaded for the 2D 'failed' mark
            const tex = new THREE.Texture(Resources().FailedNodeMark.texture.source.resource);
            tex.colorSpace = THREE.SRGBColorSpace;
            tex.needsUpdate = true;
            this._failedMarkTexture = tex;
        }
        return this._failedMarkTexture;
    }

    // ---------------------------------------------------------------- FieldRenderer interface

    addNode(state) {
        const view = new ThreeNodeView(this, state);
        if (this.building !== null) {
            view.group.visible = this.building.isHeightVisible(view.group.position.y);
        }
        this._views[state.id] = view;
        this._nodesGroup.add(view.group);
        this._labelsLayer.addChild(view.label);
    }

    removeNode(state) {
        const view = this._views[state.id];
        if (view) {
            delete this._views[state.id];
            this._nodesGroup.remove(view.group);
            this._labelsLayer.removeChild(view.label);
            view.destroy();
        }
    }

    updateNode(state) {
        this._views[state.id].redraw();
    }

    moveNode(state) {
        const view = this._views[state.id];
        view.onPositionChanged();
        if (this.building !== null) {
            view.group.visible = this.building.isHeightVisible(view.group.position.y);
        }
    }

    setSelectedNode(state) {
        for (let id in this._views) {
            this._views[id].setSelected(state !== null && Number(id) === state.id);
        }
        this._selected = state;
    }

    setPartitionVisible(visible) {
        this.partitionVisible = visible;
        for (let id in this._views) {
            this._views[id].setPartitionVisible(visible);
        }
    }

    applySkin() {
        for (let id in this._views) {
            this._views[id].redraw();
        }
    }

    drawLinks(links) {
        this._linksGroup.children.forEach((line) => {
            line.geometry.dispose();
            line.material.dispose();
        });
        this._linksGroup.clear();
        // one LineSegments per link style; WebGL lines are 1 px wide, so a selected link is
        // shown by full opacity, the others are lighter.
        const skin = Skin();
        const groups = {};
        for (let link of links) {
            const from = this._views[link.from.id];
            const to = this._views[link.to.id];
            if (!from || !to || !from.group.visible || !to.group.visible) {
                continue;
            }
            const style = skin.linkStyle(link.kind, link.selected);
            const key = style.color + ":" + link.selected;
            if (!(key in groups)) {
                groups[key] = {style: style, selected: link.selected, points: []};
            }
            groups[key].points.push(from.group.position, to.group.position);
        }
        for (let key in groups) {
            const {style, selected, points} = groups[key];
            const geom = new THREE.BufferGeometry().setFromPoints(points);
            const mat = new THREE.LineBasicMaterial({color: style.color, transparent: !selected, opacity: selected ? 1.0 : 0.6});
            this._linksGroup.add(new THREE.LineSegments(geom, mat));
        }
    }

    showBroadcast(src, mvInfo) {
        this._addMessage(new BroadcastMessage(this, this._views[src.id], mvInfo));
    }

    showUnicast(src, dst, mvInfo) {
        this._addMessage(new UnicastMessage(this, this._views[src.id], dst !== null ? this._views[dst.id] : null, mvInfo));
    }

    showAck(src, mvInfo) {
        this._addMessage(new AckMessage(this, this._views[src.id], mvInfo));
    }

    _addMessage(msg) {
        this._messagesGroup.add(msg.mesh);
        this._messages.push(msg);
    }

    update(dt) {
        if (!this._framed && (Object.keys(this._views).length > 0 || this.building !== null)) {
            this.resetView();
        }
        this._updateDrag(dt);
        this._messages = this._messages.filter((msg) => {
            if (msg.update(dt)) {
                return true;
            }
            this._messagesGroup.remove(msg.mesh);
            msg.destroy();
            return false;
        });
        this._controls.update();
        this._three.resetState();
        this._three.render(this._scene, this._camera);
        this._pixi.resetState();
        this._updateLabels();
    }

    onResize(width, height) {
        this._three.setSize(width, height, false);
        this._camera.aspect = width / height;
        this._camera.updateProjectionMatrix();
    }

    /**
     * Keys: 't' top view (the same picture as the 2D skins), 'r' reset to the default 3D view,
     * 'f' frame all nodes; with a building: '1'..'9' toggle a floor, '0' shows all floors.
     */
    onKeyDown(e) {
        switch (e.key) {
            case 't':
                this.topView();
                return true;
            case 'r':
                this.resetView();
                return true;
            case 'f':
                this.frameNodes();
                return true;
            case '0':
                if (this.building !== null) {
                    this.building.showAllFloors();
                    this._applyFloorVisibility();
                    return true;
                }
                return false;
            default:
                if (this.building !== null && e.key >= '1' && e.key <= '9') {
                    if (!this.building.toggleFloor(Number(e.key) - 1)) {
                        return false;
                    }
                    this._applyFloorVisibility();
                    return true;
                }
                return false;
        }
    }

    nodeAt(global) {
        const hit = this._pick(global);
        return hit !== null ? hit.state : null;
    }

    destroy() {
        this._destroyed = true;
        const root = this.vis.root;
        root.off('pointerdown', this._onPointerDown);
        root.off('globalpointermove', this._onPointerMove);
        root.off('pointerup', this._onPointerUp);
        root.off('pointerupoutside', this._onPointerUp);
        this._controls.dispose();
        for (let id in this._views) {
            this._views[id].destroy();
        }
        this._views = {};
        this._messages.forEach((msg) => msg.destroy());
        this._messages = [];
        for (let key in this._geometries) {
            this._geometries[key].dispose();
        }
        if (this._failedMarkTexture) {
            this._failedMarkTexture.dispose();
        }
        if (this.building !== null) {
            this.building.dispose();
        }
        this._three.dispose();
        this._pixi.background.clearBeforeRender = true;
        this._pixi.resetState();
        this._backLayer.destroy({children: true});
        this._frontLayer.destroy({children: true});
    }

    // ---------------------------------------------------------------- camera

    /**
     * @returns {{center: THREE.Vector3, extent: number}} bounds of all nodes (three.js coordinates)
     */
    _nodeBounds() {
        const box = new THREE.Box3();
        for (let id in this._views) {
            box.expandByPoint(this._views[id].group.position);
        }
        if (this.building !== null && !this.building.bounds.isEmpty()) {
            box.union(this.building.bounds);
        }
        if (box.isEmpty()) {
            box.set(new THREE.Vector3(0, 0, 0), new THREE.Vector3(1000, 0, 1000));
        }
        const center = box.getCenter(new THREE.Vector3());
        const size = box.getSize(new THREE.Vector3());
        return {center: center, extent: Math.max(size.x, size.z, 400)};
    }

    /**
     * Default 3D view: from the south (2D: bottom) side, looking down at the nodes.
     */
    resetView() {
        const {center, extent} = this._nodeBounds();
        this._controls.target.copy(center);
        this._camera.position.set(center.x, center.y + extent * 0.9, center.z + extent * 1.2);
        this._controls.update();
        this._framed = true;
    }

    /**
     * Top view: the same orientation as the 2D skins.
     */
    topView() {
        const {center, extent} = this._nodeBounds();
        this._controls.target.copy(center);
        this._camera.position.set(center.x, center.y + extent * 2.0, center.z + 0.01);
        this._controls.update();
        this._framed = true;
    }

    /**
     * Move the camera so that all nodes are in view, keeping its direction.
     */
    frameNodes() {
        const {center, extent} = this._nodeBounds();
        const dir = this._camera.position.clone().sub(this._controls.target).normalize();
        if (dir.lengthSq() === 0) {
            this.resetView();
            return;
        }
        this._controls.target.copy(center);
        this._camera.position.copy(center).addScaledVector(dir, extent * 1.25);
        this._controls.update();
        this._framed = true;
    }

    // ---------------------------------------------------------------- projection and picking

    /**
     * @returns {{x: number, y: number, visible: boolean}} screen position (canvas px) of a node
     */
    screenPositionOf(state) {
        const view = this._views[state.id];
        const v = view.group.position.clone().project(this._camera);
        return {x: (v.x + 1) / 2 * this._pixi.screen.width, y: (1 - v.y) / 2 * this._pixi.screen.height, visible: v.z < 1};
    }

    _updateLabels() {
        const labelOffset = Skin().labelOffset;
        const root = this.vis.root.position;
        for (let id in this._views) {
            const view = this._views[id];
            const s = this.screenPositionOf(view.state);
            view.label.position.set(s.x - root.x + labelOffset, s.y - root.y + labelOffset);
            view.label.visible = s.visible && view.group.visible;
        }
    }

    _setRay(global) {
        const ndc = new THREE.Vector2(global.x / this._pixi.screen.width * 2 - 1, -(global.y / this._pixi.screen.height * 2 - 1));
        this._raycaster.setFromCamera(ndc, this._camera);
    }

    /**
     * @returns {ThreeNodeView|null} the node under the pointer position (canvas px)
     */
    _pick(global) {
        this._setRay(global);
        const bodies = Object.values(this._views).map((view) => view.body);
        const hits = this._raycaster.intersectObjects(bodies, false);
        return hits.length > 0 ? this._views[hits[0].object.userData.nodeId] : null;
    }

    // ---------------------------------------------------------------- pointer: select, drag, orbit

    _pointerDown(e) {
        if (e.target !== this.vis.root) {
            return; // a HUD element
        }
        const view = this._pick(e.global);
        if (view === null) {
            this._controls.enableRotate = true;
            this._controls.enablePan = true;
            return;
        }
        this.vis.setSelectedNode(view.id);
        this._press = {view: view, x: e.global.x, y: e.global.y};
    }

    _pointerMove(e) {
        if (this._press !== null && this._drag === null) {
            if (Math.abs(e.global.x - this._press.x) >= DRAG_START_DISTANCE || Math.abs(e.global.y - this._press.y) >= DRAG_START_DISTANCE) {
                this._startDrag(this._press.view, this._press, e.altKey);
            }
        }
        if (this._drag !== null) {
            if (e.altKey !== this._drag.vertical) {
                this._setDragPlane(this._drag, e.altKey, e.global); // re-anchor in the other plane
            }
            this._setRay(e.global);
            const hit = new THREE.Vector3();
            if (this._raycaster.ray.intersectPlane(this._drag.plane, hit) !== null) {
                const pos = this._drag.view.group.position;
                if (this._drag.vertical) {
                    pos.y = Math.round(hit.y - this._drag.offset.y);
                } else {
                    pos.x = Math.round(hit.x - this._drag.offset.x);
                    pos.z = Math.round(hit.z - this._drag.offset.z);
                }
                this._drag.view._updateDropLine();
                this._drag.moved = true;
            }
        }
    }

    _pointerUp(e) {
        this._controls.enableRotate = false;
        this._controls.enablePan = false;
        this._press = null;
        if (this._drag !== null) {
            const drag = this._drag;
            this._drag = null;
            drag.view.dragging = false;
            const p = fromThree(drag.view.group.position);
            this.vis.ctrlMoveNodeTo(drag.view.id, p.x, p.y, p.z, (err, resp) => {
                if (err !== null) {
                    drag.view.onPositionChanged();
                }
            });
        }
    }

    /**
     * Start dragging `view` from pointer position `at`: in its horizontal plane, or vertically
     * when the Alt key is held.
     */
    _startDrag(view, at, vertical) {
        const drag = {view: view, plane: new THREE.Plane(), offset: new THREE.Vector3(), vertical: false, moved: false, timer: 0};
        if (!this._setDragPlane(drag, vertical, at)) {
            return;
        }
        view.dragging = true;
        this._drag = drag;
        this._press = null;
    }

    /**
     * Set the plane that the pointer is projected onto while dragging, through the node's
     * current position: horizontal, or vertical and facing the camera for a height drag. The
     * offset between the pointer's projection and the node is re-anchored at `at`.
     * @returns {boolean} false if the pointer ray misses the plane (drag not possible)
     */
    _setDragPlane(drag, vertical, at) {
        const pos = drag.view.group.position;
        if (vertical) {
            const normal = this._camera.getWorldDirection(new THREE.Vector3());
            normal.y = 0;
            if (normal.lengthSq() < 1e-6) { // looking straight down: any vertical plane will do
                normal.set(0, 0, 1);
            }
            drag.plane.setFromNormalAndCoplanarPoint(normal.normalize(), pos);
        } else {
            drag.plane.set(UP, -pos.y);
        }
        this._setRay(at);
        const hit = new THREE.Vector3();
        if (this._raycaster.ray.intersectPlane(drag.plane, hit) === null) {
            return false;
        }
        drag.vertical = vertical;
        drag.offset.copy(hit).sub(pos);
        return true;
    }

    _updateDrag(dt) {
        if (this._drag === null) {
            return;
        }
        this._drag.timer += dt;
        if (this._drag.timer >= DRAG_MOVE_INTERVAL && this._drag.moved) {
            this._drag.timer = 0;
            this._drag.moved = false;
            const p = fromThree(this._drag.view.group.position);
            this.vis.ctrlMoveNodeTo(this._drag.view.id, p.x, p.y, p.z, () => {});
        }
    }
}
