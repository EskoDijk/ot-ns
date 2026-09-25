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
// Building: the 3D geometry of a floor plan (see etc/floorplans/README.md for the JSON format),
// drawn by ThreeFieldRenderer as floor slabs and translucent walls. All plan dimensions are in
// meters; they are converted to OTNS units with the plan's unitsPerMeter and origin, and to
// three.js coordinates like the nodes: OTNS (x, y, z) -> three.js (x, z, y).
//
// A plan may name a glTF model ('model' entry) for the looks of the building; the plan's own
// slabs and walls are then hidden by default (key 'w' shows them, to check the alignment). The
// model is in meters, y up, with its z axis along the plan's y axis, like the plan itself.

import * as THREE from 'three';
import {GLTFLoader} from 'three/addons/loaders/GLTFLoader.js';

const DEFAULT_UNITS_PER_METER = 10;
const DEFAULT_WALL_THICKNESS = 0.2; // m
const DEFAULT_WALL_HEIGHT = 3.0;    // m
const DEFAULT_DOOR_HEIGHT = 2.1;    // m
const SLAB_THICKNESS = 0.15;        // m
const COLOR_WALL = 0x90a4ae;
const WALL_OPACITY = 0.35;
const COLOR_SLAB = 0xcfd8dc;
const SLAB_OPACITY = 0.9;
const RENDER_ORDER_SLAB = 1;        // transparent parts: slabs first, then walls (both after the nodes)
const RENDER_ORDER_WALL = 2;

export default class Building {
    /**
     * @param plan the parsed floor plan JSON
     */
    constructor(plan) {
        this.plan = plan;
        this.name = plan.name || "";
        this.unitsPerMeter = plan.unitsPerMeter || DEFAULT_UNITS_PER_METER;
        this.origin = plan.origin || [0, 0];
        this.nodeScale = plan.nodeScale || 1.0;
        this.wallThickness = plan.wallThickness || DEFAULT_WALL_THICKNESS;
        this.doorHeight = plan.doorHeight || DEFAULT_DOOR_HEIGHT;
        this.group = new THREE.Group();
        this.floors = []; // one Group per floor, in the order of the plan
        this._floorRanges = []; // [bottom, top] in three.js y (OTNS z units) per floor
        this._floorVisible = [];
        this.bounds = new THREE.Box3();
        this.model = null;            // the glTF scene, once loaded
        this.modelFloors = [];        // glTF nodes matched to the plan's floors by name (or null)
        this.planVisible = !plan.model; // the plan's own slabs and walls are hidden when a model is used

        // Translucent parts must not write depth: three.js sorts transparent objects by distance,
        // and a depth-writing slab drawn before a wall behind it would hide that wall, depending
        // on the camera angle (walls "popping"). Slabs are drawn before walls (renderOrder).
        this._wallMaterial = new THREE.MeshLambertMaterial({
            color: COLOR_WALL, transparent: true, opacity: WALL_OPACITY, depthWrite: false, side: THREE.DoubleSide,
        });
        this._slabMaterial = new THREE.MeshLambertMaterial({
            color: COLOR_SLAB, transparent: true, opacity: SLAB_OPACITY, depthWrite: false, side: THREE.DoubleSide,
        });
        this._geometries = [];

        for (let floor of plan.floors || []) {
            this._addFloor(floor);
        }
    }

    /**
     * @returns {THREE.Vector3} three.js position of a point (mx, my) meters on the plan at height mz meters
     */
    toScene(mx, my, mz) {
        const u = this.unitsPerMeter;
        return new THREE.Vector3(this.origin[0] + mx * u, mz * u, this.origin[1] + my * u);
    }

    _addFloor(floor) {
        const group = new THREE.Group();
        group.name = floor.name || ("floor " + (this.floors.length + 1));
        const elevation = floor.elevation || 0;
        const height = floor.height || DEFAULT_WALL_HEIGHT;
        const [w, d] = floor.outline || [0, 0];
        if (w > 0 && d > 0) {
            // floor slab, with its top at the floor's elevation
            this._addBox(group, w, SLAB_THICKNESS, d, this.toScene(w / 2, d / 2, elevation - SLAB_THICKNESS / 2), 0, this._slabMaterial);
            // exterior walls
            this._addWall(group, [0, 0], [w, 0], this.wallThickness, height, elevation, []);
            this._addWall(group, [w, 0], [w, d], this.wallThickness, height, elevation, []);
            this._addWall(group, [w, d], [0, d], this.wallThickness, height, elevation, []);
            this._addWall(group, [0, d], [0, 0], this.wallThickness, height, elevation, []);
            this.bounds.expandByPoint(this.toScene(0, 0, elevation));
            this.bounds.expandByPoint(this.toScene(w, d, elevation + height));
        }
        for (let wall of floor.walls || []) {
            this._addWall(group, wall.from, wall.to, wall.thickness || this.wallThickness, wall.height || height, elevation, wall.openings || []);
        }
        group.visible = this.planVisible;
        this.floors.push(group);
        this._floorVisible.push(true);
        this._floorRanges.push([elevation * this.unitsPerMeter, (elevation + height) * this.unitsPerMeter]);
        this.group.add(group);
    }

    /**
     * Load the plan's glTF model, if any, and add it to the building.
     * @param baseUrl URL that a relative model URL is resolved against
     * @returns {Promise<boolean>} true if a model was loaded, false if the plan has none
     */
    loadModel(baseUrl) {
        const spec = this.plan.model;
        if (!spec || !spec.url) {
            return Promise.resolve(false);
        }
        const url = new URL(spec.url, baseUrl).href;
        return new Promise((resolve, reject) => {
            new GLTFLoader().load(url, (gltf) => {
                const model = gltf.scene;
                const u = this.unitsPerMeter * (spec.scale || 1);
                const pos = spec.position || [0, 0, 0];
                model.scale.set(u, u, u);
                model.rotation.y = -(spec.rotation || 0) * Math.PI / 180;
                model.position.copy(this.toScene(pos[0], pos[1], pos[2]));
                model.traverse((obj) => {
                    if (obj.isMesh) { // translucent parts must not write depth, see the plan materials
                        const materials = Array.isArray(obj.material) ? obj.material : [obj.material];
                        materials.forEach((m) => {
                            if (m.transparent) {
                                m.depthWrite = false;
                            }
                        });
                    }
                });
                // top-level model nodes named like a plan floor follow that floor's visibility.
                // GLTFLoader sanitizes node names (e.g. spaces become '_'), so compare loosely.
                const key = (name) => name.toLowerCase().replace(/[^a-z0-9]/g, '');
                this.modelFloors = (this.plan.floors || []).map((floor) => {
                    const name = key(floor.name || "");
                    return name ? (model.children.find((c) => key(c.name) === name) || null) : null;
                });
                this.model = model;
                this.group.add(model);
                this.bounds.union(new THREE.Box3().setFromObject(model));
                this._applyVisibility();
                resolve(true);
            }, undefined, (err) => reject(new Error("glTF model " + url + ": " + (err.message || err))));
        });
    }

    _applyVisibility() {
        this.floors.forEach((g, i) => g.visible = this.planVisible && this._floorVisible[i]);
        this.modelFloors.forEach((node, i) => {
            if (node !== null) {
                node.visible = this._floorVisible[i];
            }
        });
    }

    /**
     * Show or hide the plan's own slabs and walls (relevant when a model is shown).
     */
    setPlanVisible(visible) {
        this.planVisible = visible;
        this._applyVisibility();
    }

    /**
     * @param y height in three.js coordinates (OTNS z units)
     * @returns {number} index of the floor whose space contains that height, or -1 if none
     */
    floorIndexAtHeight(y) {
        for (let i = this._floorRanges.length - 1; i >= 0; i--) {
            const [bottom, top] = this._floorRanges[i];
            if (y >= bottom && y < top) {
                return i;
            }
        }
        return -1;
    }

    /**
     * @returns {boolean} whether a node at height `y` (three.js) is on a visible floor, or outside the building
     */
    isHeightVisible(y) {
        const i = this.floorIndexAtHeight(y);
        return i < 0 || this._floorVisible[i];
    }

    /**
     * A wall from `from` to `to` (meters), split by its door openings; a lintel is kept above each.
     */
    _addWall(group, from, to, thickness, height, elevation, openings) {
        const dx = to[0] - from[0];
        const dy = to[1] - from[1];
        const length = Math.sqrt(dx * dx + dy * dy);
        if (length <= 0) {
            return;
        }
        const angle = -Math.atan2(dy, dx); // rotation about the vertical axis, see _addBox()
        const along = (t) => [from[0] + dx * t / length, from[1] + dy * t / length];
        // wall pieces between the openings, in order along the wall
        const gaps = openings.map((o) => [o.at - o.width / 2, o.at + o.width / 2]).sort((a, b) => a[0] - b[0]);
        let start = 0;
        for (let [gapStart, gapEnd] of gaps) {
            gapStart = Math.max(gapStart, 0);
            gapEnd = Math.min(gapEnd, length);
            if (gapStart > start) {
                this._addWallPiece(group, along, start, gapStart, thickness, height, elevation, angle);
            }
            if (gapEnd > gapStart && height > this.doorHeight) { // lintel
                const mid = along((gapStart + gapEnd) / 2);
                this._addBox(group, gapEnd - gapStart, height - this.doorHeight, thickness,
                    this.toScene(mid[0], mid[1], elevation + this.doorHeight + (height - this.doorHeight) / 2), angle, this._wallMaterial);
            }
            start = Math.max(start, gapEnd);
        }
        if (length > start) {
            this._addWallPiece(group, along, start, length, thickness, height, elevation, angle);
        }
    }

    _addWallPiece(group, along, t0, t1, thickness, height, elevation, angle) {
        const mid = along((t0 + t1) / 2);
        this._addBox(group, t1 - t0, height, thickness, this.toScene(mid[0], mid[1], elevation + height / 2), angle, this._wallMaterial);
    }

    /**
     * Add a box of `w` (along the wall) x `h` (vertical) x `d` meters, centered at `center`
     * (three.js coordinates), rotated by `angle` about the vertical axis.
     */
    _addBox(group, w, h, d, center, angle, material) {
        const u = this.unitsPerMeter;
        const geom = new THREE.BoxGeometry(w * u, h * u, d * u);
        this._geometries.push(geom);
        const mesh = new THREE.Mesh(geom, material);
        mesh.position.copy(center);
        mesh.rotation.y = angle;
        mesh.renderOrder = material === this._slabMaterial ? RENDER_ORDER_SLAB : RENDER_ORDER_WALL;
        group.add(mesh);
    }

    /**
     * Show or hide floor `index` (0 = lowest in the plan).
     * @returns {boolean} false if there is no such floor
     */
    setFloorVisible(index, visible) {
        if (index < 0 || index >= this.floors.length) {
            return false;
        }
        this._floorVisible[index] = visible;
        this._applyVisibility();
        return true;
    }

    toggleFloor(index) {
        if (index < 0 || index >= this.floors.length) {
            return false;
        }
        return this.setFloorVisible(index, !this._floorVisible[index]);
    }

    showAllFloors() {
        this._floorVisible.fill(true);
        this._applyVisibility();
    }

    dispose() {
        this._geometries.forEach((g) => g.dispose());
        this._wallMaterial.dispose();
        this._slabMaterial.dispose();
        if (this.model !== null) {
            this.model.traverse((obj) => {
                if (obj.isMesh) {
                    obj.geometry.dispose();
                    (Array.isArray(obj.material) ? obj.material : [obj.material]).forEach((m) => m.dispose());
                }
            });
        }
    }
}
