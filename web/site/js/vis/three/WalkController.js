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
// WalkController: first-person navigation for the 3D field renderer's walk mode. The mouse looks
// around (pointer lock), W/A/S/D or the arrow keys move, Shift runs. Collision with the building's
// meshes is ray based: rays at knee and hip height in the movement direction stop the player at
// walls (sliding along them), and a probe ray down from above the feet finds the floor, so that
// steps up to STEP_HEIGHT_M (stairs) are climbed and gravity applies over a drop. This needs no
// spatial structure (a three.js Octree of CAD-style geometry with huge triangles subdivides
// without end), at the cost of testing all triangles per ray, which is fine for buildings of tens
// of thousands of triangles. Without a building the player walks on the floor plane y = 0.
// All distances are OTNS units; the controller is told the building's units per meter.

import * as THREE from 'three';
import {PointerLockControls} from 'three/addons/controls/PointerLockControls.js';

const EYE_HEIGHT_M = 1.6;
const BODY_RADIUS_M = 0.3;      // distance kept from walls
const WALL_RAY_HEIGHTS_M = [0.5, 1.0, 1.5]; // above the feet; all above STEP_HEIGHT so steps are not walls
const STEP_HEIGHT_M = 0.35;     // highest step that is climbed without stopping
const FLOOR_PROBE_M = 0.6;      // how far below the feet the floor is looked for before falling
const WALK_SPEED_M = 3.0;       // m/s
const RUN_FACTOR = 2.0;
const GRAVITY_M = 9.8;          // m/s^2
const MAX_STEP_DT = 0.05;       // s; a frame is split into sub-steps of at most this length
const MOVE_KEYS = {
    KeyW: 'forward', ArrowUp: 'forward', KeyS: 'back', ArrowDown: 'back',
    KeyA: 'left', ArrowLeft: 'left', KeyD: 'right', ArrowRight: 'right',
};
const DOWN = new THREE.Vector3(0, -1, 0);

export default class WalkController {
    /**
     * @param camera the camera to move
     * @param domElement the canvas, for pointer lock
     * @param unitsPerMeter OTNS units per meter
     * @param collisionRoot {THREE.Object3D|null} meshes to collide with, or null for a flat floor at y=0
     * @param feet {THREE.Vector3} start position of the player's feet
     */
    constructor(camera, domElement, unitsPerMeter, collisionRoot, feet) {
        this.camera = camera;
        this.u = unitsPerMeter;
        this.controls = new PointerLockControls(camera, domElement);
        this.meshes = [];   // collision meshes with their world bounding boxes: {mesh, box}
        if (collisionRoot !== null) {
            collisionRoot.updateMatrixWorld(true);
            collisionRoot.traverse((obj) => {
                if (obj.isMesh) {
                    this.meshes.push({mesh: obj, box: new THREE.Box3().setFromObject(obj)});
                }
            });
        }
        this.raycaster = new THREE.Raycaster();
        this.feet = feet.clone();
        this.verticalSpeed = 0;
        this.onFloor = false;
        this.keys = {};
        this.running = false;
        this.onExit = null; // called when the pointer lock ends (Escape)
        this._onKeyDown = (e) => this._key(e, true);
        this._onKeyUp = (e) => this._key(e, false);
        this._onUnlock = () => {
            if (this.onExit !== null) {
                this.onExit();
            }
        };
        window.addEventListener('keydown', this._onKeyDown);
        window.addEventListener('keyup', this._onKeyUp);
        this.controls.addEventListener('unlock', this._onUnlock);
        this._settleOnFloor();
        this._placeCamera();
    }

    /**
     * @returns {boolean} whether the key event is a movement key of this controller
     */
    static isMoveKey(e) {
        return e.code in MOVE_KEYS || e.code === 'ShiftLeft' || e.code === 'ShiftRight';
    }

    /**
     * Request the pointer lock for mouse look. Browsers require a user gesture (a click or key
     * press) for it; the returned promise rejects otherwise.
     * @returns {Promise}
     */
    lock() {
        try {
            const p = this.controls.domElement.requestPointerLock();
            return p && p.then ? p : Promise.resolve();
        } catch (err) {
            return Promise.reject(err);
        }
    }

    get isLocked() {
        return this.controls.isLocked;
    }

    _key(e, down) {
        if (e.code in MOVE_KEYS) {
            this.keys[MOVE_KEYS[e.code]] = down;
        } else if (e.code === 'ShiftLeft' || e.code === 'ShiftRight') {
            this.running = down;
        }
    }

    update(dt) {
        const steps = Math.max(1, Math.ceil(dt / MAX_STEP_DT));
        const stepDt = dt / steps;
        for (let i = 0; i < steps; i++) {
            this._step(stepDt);
        }
        this._placeCamera();
    }

    _step(dt) {
        // horizontal movement in the camera's yaw frame, at a fixed walking speed
        const forward = this.camera.getWorldDirection(new THREE.Vector3());
        forward.y = 0;
        forward.normalize();
        const right = new THREE.Vector3().crossVectors(forward, this.camera.up).normalize();
        const dir = new THREE.Vector3();
        if (this.keys.forward) dir.add(forward);
        if (this.keys.back) dir.sub(forward);
        if (this.keys.right) dir.add(right);
        if (this.keys.left) dir.sub(right);
        if (dir.lengthSq() > 0) {
            const speed = WALK_SPEED_M * this.u * (this.running ? RUN_FACTOR : 1);
            this._moveHorizontally(dir.normalize().multiplyScalar(speed * dt));
        }
        // vertical: stand on the floor found below, else fall
        if (this.onFloor) {
            this.verticalSpeed = 0;
        } else {
            this.verticalSpeed -= GRAVITY_M * this.u * dt;
            this.feet.y += this.verticalSpeed * dt;
        }
        this._settleOnFloor();
    }

    /**
     * Move the feet by `delta` (horizontal), stopped by walls and sliding along them.
     */
    _moveHorizontally(delta) {
        let remaining = delta;
        for (let attempt = 0; attempt < 2 && remaining.lengthSq() > 0; attempt++) {
            const hit = this._wallHit(remaining);
            if (hit === null) {
                this.feet.add(remaining);
                return;
            }
            // advance up to the wall, then slide with what is left of the movement
            const length = remaining.length();
            const dirN = remaining.clone().divideScalar(length);
            const allowed = Math.max(0, hit.distance - BODY_RADIUS_M * this.u);
            this.feet.addScaledVector(dirN, Math.min(allowed, length));
            const n = hit.normal;
            n.y = 0;
            if (n.lengthSq() === 0) {
                return;
            }
            n.normalize();
            remaining = dirN.multiplyScalar(length - Math.min(allowed, length));
            remaining.addScaledVector(n, -remaining.dot(n));
        }
    }

    /**
     * @returns {{distance: number, normal: THREE.Vector3}|null} the nearest wall hit within
     *          body radius plus the movement length, from rays at several heights above the feet
     */
    _wallHit(delta) {
        if (this.meshes.length === 0) {
            return null;
        }
        const length = delta.length();
        const dir = delta.clone().divideScalar(length);
        this.raycaster.far = length + BODY_RADIUS_M * this.u;
        let nearest = null;
        for (let h of WALL_RAY_HEIGHTS_M) {
            const origin = this.feet.clone();
            origin.y += h * this.u;
            const hits = this._cast(origin, dir, this.raycaster.far);
            if (hits.length > 0 && (nearest === null || hits[0].distance < nearest.distance)) {
                const normal = hits[0].face.normal.clone().transformDirection(hits[0].object.matrixWorld);
                if (normal.dot(dir) > 0) {
                    normal.negate(); // hit from the back side
                }
                nearest = {distance: hits[0].distance, normal: normal};
            }
        }
        return nearest;
    }

    /**
     * Put the feet on the floor found by a ray from just above the feet downwards: a step up to
     * STEP_HEIGHT is climbed, a drop up to FLOOR_PROBE is descended, further away means falling.
     */
    _settleOnFloor() {
        if (this.meshes.length === 0) {
            this.onFloor = this.feet.y <= 0;
            if (this.feet.y < 0) {
                this.feet.y = 0;
            }
            return;
        }
        const origin = this.feet.clone();
        origin.y += STEP_HEIGHT_M * this.u;
        const hits = this._cast(origin, DOWN, (STEP_HEIGHT_M + FLOOR_PROBE_M) * this.u);
        if (hits.length > 0) {
            this.feet.y = hits[0].point.y;
            this.onFloor = true;
        } else {
            this.onFloor = false;
        }
    }

    /**
     * Ray cast against the collision meshes whose world bounding box the ray segment enters;
     * this skips most meshes (e.g. other storeys) before their triangles are tested.
     * @returns intersections sorted by distance
     */
    _cast(origin, dir, far) {
        this.raycaster.set(origin, dir);
        this.raycaster.far = far;
        const ray = this.raycaster.ray;
        const entry = new THREE.Vector3();
        const candidates = [];
        for (let {mesh, box} of this.meshes) {
            if (box.containsPoint(origin) || (ray.intersectBox(box, entry) !== null && entry.distanceTo(origin) <= far)) {
                candidates.push(mesh);
            }
        }
        return candidates.length > 0 ? this.raycaster.intersectObjects(candidates, false) : [];
    }

    _placeCamera() {
        this.camera.position.set(this.feet.x, this.feet.y + EYE_HEIGHT_M * this.u, this.feet.z);
    }

    dispose() {
        window.removeEventListener('keydown', this._onKeyDown);
        window.removeEventListener('keyup', this._onKeyUp);
        this.controls.removeEventListener('unlock', this._onUnlock);
        this.onExit = null;
        if (this.controls.isLocked) {
            this.controls.unlock();
        }
        this.controls.dispose();
    }
}
