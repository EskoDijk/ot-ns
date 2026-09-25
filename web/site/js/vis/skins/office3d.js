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
// The 'office3d' skin: a 3D view of the field, drawn by three/ThreeFieldRenderer.js. Node shapes
// follow the classic skin, node colors the Thread diagram conventions of the thread skin:
//   - Router/Leader: hexagonal prism, Thread orange for a Router, Thread grey for the Leader.
//     Border Router: cube, black (Thread grey when it is the Leader).
//   - End Devices: sphere; light orange-red for a router-capable one (REED), light grey for
//     the others (FED, MED, SED, SSED). MTDs are smaller: a MED translucent, SED/SSED wireframe.
//   - Detached, disabled and failed nodes are dark grey.
//   - The partition color is a band around the node.
// The link, message and selection styles, and everything drawn by Pixi (labels, HUD), are
// those of the classic skin.

import {OtDeviceRole} from '../../proto/visualize_grpc_pb'
import ClassicSkin from "./classic";
import {isBorderRouter} from "./Skin";
import {COLOR_THREAD_ORANGE, COLOR_THREAD_GREY, COLOR_BR_BLACK, isRouterCapable} from "./thread";

const COLOR_ROUTER = COLOR_THREAD_ORANGE;
const COLOR_LEADER = COLOR_THREAD_GREY;
const COLOR_BR = COLOR_BR_BLACK;
const COLOR_REED = 0xffa07a;        // light orange-red: router-capable End Device
const COLOR_END_DEVICE = 0xb0bec5;  // light grey: FED and MTDs
const COLOR_DETACHED = 0x546e7a;
const COLOR_DISABLED = 0x757575;
const COLOR_FAILED = 0x757575;

// about 3/4 of the classic skin's 2D shape sizes
const HEXAGONAL_PRISM_RADIUS = 16;
const CUBE_RADIUS = 16;
const SPHERE_RADIUS = 15;
const MED_SPHERE_RADIUS = 13;
const SED_SPHERE_RADIUS = 12;
const NODE_SELECTION_BOX_SIZE = 45;

function nodeColor(state) {
    if (state.failed) {
        return COLOR_FAILED;
    }
    switch (state.role) {
        case OtDeviceRole.OT_DEVICE_ROLE_LEADER:
            return COLOR_LEADER;
        case OtDeviceRole.OT_DEVICE_ROLE_ROUTER:
            return isBorderRouter(state.type) ? COLOR_BR : COLOR_ROUTER;
        case OtDeviceRole.OT_DEVICE_ROLE_CHILD:
            if (isBorderRouter(state.type)) {
                return COLOR_BR;
            }
            return isRouterCapable(state.type, state.nodeMode) ? COLOR_REED : COLOR_END_DEVICE;
        case OtDeviceRole.OT_DEVICE_ROLE_DETACHED:
            return COLOR_DETACHED;
        default:
            return COLOR_DISABLED;
    }
}

export default class Office3DSkin extends ClassicSkin {
    get renderer() {
        return 'three';
    }

    selectionStyle() {
        return {...super.selectionStyle(), boxSize: NODE_SELECTION_BOX_SIZE};
    }

    node3DStyle(state) {
        const color = nodeColor(state);
        const isRouterRole = state.role === OtDeviceRole.OT_DEVICE_ROLE_LEADER || state.role === OtDeviceRole.OT_DEVICE_ROLE_ROUTER;
        if (isBorderRouter(state.type)) {
            return {shape: 'cube', radius: CUBE_RADIUS, color: color, opacity: 1.0, wireframe: false};
        }
        if (isRouterRole) {
            return {shape: 'hexprism', radius: HEXAGONAL_PRISM_RADIUS, color: color, opacity: 1.0, wireframe: false};
        }
        if (state.nodeMode.getFullThreadDevice()) {
            return {shape: 'sphere', radius: SPHERE_RADIUS, color: color, opacity: 1.0, wireframe: false};
        }
        if (state.nodeMode.getRxOnWhenIdle()) {
            return {shape: 'sphere', radius: MED_SPHERE_RADIUS, color: color, opacity: 0.6, wireframe: false};
        }
        return {shape: 'sphere', radius: SED_SPHERE_RADIUS, color: color, opacity: 1.0, wireframe: true};
    }
}
