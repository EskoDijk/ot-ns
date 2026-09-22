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
// The 'classic' skin: the original OTNS look. Nodes are solid shapes colored by role
// (Leader red, Router blue, Child green, Detached/Disabled grey) with a smaller copy of
// the shape on top in the partition color:
//   - Router/Leader: hexagon. Border Router: square. Other nodes: circle.
//   - MTD: dashed circle: 4 dashes = MED, 6 dashes = SSED, 8 dashes = SED.
//   - Links: green between parent and child, blue for router-table links.
//   - Messages: blue expanding circle for broadcast, orange hexagon for unicast, green
//     triangle for ACK.

import {OtDeviceRole} from '../../proto/visualize_grpc_pb'
import {Resources} from "../resources";
import Skin, {tintedSprite} from "./Skin";

const COLOR_LEADER = 0xc62828;
const COLOR_ROUTER = 0x1565c0;
const COLOR_CHILD = 0x4caf50;
const COLOR_DETACHED = 0x546e7a;
const COLOR_DISABLED = 0x757575;
const COLOR_FAILED = 0x757575;
const COLOR_NODE_SELECTION = 0x2e7d32;
const COLOR_RANGE_FILL = 0x98ee99;
const COLOR_RANGE_STROKE = 0x338a3e;
const COLOR_LINK_PARENT_CHILD = 0x8bc34a;
const COLOR_LINK_ROUTER = 0x1976d2;
const COLOR_UNICAST_MESSAGE = 0xff8f00;
const COLOR_BROADCAST_MESSAGE = 0x1565c0;
const COLOR_ACK_MESSAGE = 0xaee571;

const NODE_SHAPE_TEXTURE_SIZE = 64;
const CIRCULAR_SHAPE_RADIUS = 20;
const HEXAGONAL_SHAPE_RADIUS = 22;
const SQUARE_SHAPE_RADIUS = 22;
const PARTITION_SHAPE_SCALE = 1 / 1.5; // partition mark size relative to the node shape
const NODE_LABEL_OFFSET = 11;
const FAILED_MARK_SCALE = 0.5;
const NODE_SELECTION_BOX_SIZE = 60;
const LINK_WIDTH = 1;
const LINK_WIDTH_SELECTED = 3;
const BROADCAST_MESSAGE_BEGIN_RADIUS = 32;
const MESSAGE_SIZE = 10;

function isRouterRole(role) {
    return role === OtDeviceRole.OT_DEVICE_ROLE_LEADER || role === OtDeviceRole.OT_DEVICE_ROLE_ROUTER;
}

function shapeRadius(state) {
    if (state.type === 'br') {
        return SQUARE_SHAPE_RADIUS;
    }
    return isRouterRole(state.role) ? HEXAGONAL_SHAPE_RADIUS : CIRCULAR_SHAPE_RADIUS;
}

// texture of the node body: role/type shape, with the MTD type shown by the dashing.
function bodyTexture(state) {
    const res = Resources();
    if (state.type === 'br') {
        return res.WhiteSolidSquare64.texture;
    }
    if (isRouterRole(state.role)) {
        return res.WhiteSolidHexagon64.texture;
    }
    if (state.nodeMode.getFullThreadDevice()) {
        return res.WhiteSolidCircle64.texture;
    }
    if (state.nodeMode.getRxOnWhenIdle()) {
        return res.WhiteDashed4Circle64.texture;
    }
    return state.type === 'ssed' ? res.WhiteDashed6Circle64.texture : res.WhiteDashed8Circle64.texture;
}

// texture of the partition mark: the solid variant of the node's shape.
function partitionTexture(state) {
    const res = Resources();
    if (state.type === 'br') {
        return res.WhiteSolidSquare64.texture;
    }
    return isRouterRole(state.role) ? res.WhiteSolidHexagon64.texture : res.WhiteSolidCircle64.texture;
}

function roleColor(state) {
    if (state.failed) {
        return COLOR_FAILED;
    }
    switch (state.role) {
        case OtDeviceRole.OT_DEVICE_ROLE_LEADER:
            return COLOR_LEADER;
        case OtDeviceRole.OT_DEVICE_ROLE_ROUTER:
            return COLOR_ROUTER;
        case OtDeviceRole.OT_DEVICE_ROLE_CHILD:
            return COLOR_CHILD;
        case OtDeviceRole.OT_DEVICE_ROLE_DETACHED:
            return COLOR_DETACHED;
        default:
            return COLOR_DISABLED;
    }
}

export default class ClassicSkin extends Skin {
    buildNodeBody(container, state) {
        const size = shapeRadius(state) * 2;
        container.addChild(tintedSprite(bodyTexture(state), NODE_SHAPE_TEXTURE_SIZE, size, roleColor(state)));
    }

    buildPartitionMark(container, state, color) {
        const size = shapeRadius(state) * 2 * PARTITION_SHAPE_SCALE;
        container.addChild(tintedSprite(partitionTexture(state), NODE_SHAPE_TEXTURE_SIZE, size, color));
    }

    nodeHitRadius(state) {
        return CIRCULAR_SHAPE_RADIUS;
    }

    get labelOffset() {
        return NODE_LABEL_OFFSET;
    }

    get failedMarkScale() {
        return FAILED_MARK_SCALE;
    }

    selectionStyle() {
        return {
            boxColor: COLOR_NODE_SELECTION, boxSize: NODE_SELECTION_BOX_SIZE, boxAlpha: 0.7,
            rangeFill: COLOR_RANGE_FILL, rangeFillAlpha: 0.2,
            rangeStroke: COLOR_RANGE_STROKE, rangeStrokeAlpha: 0.7,
        };
    }

    linkStyle(kind, selected) {
        return {
            color: kind === 'child' ? COLOR_LINK_PARENT_CHILD : COLOR_LINK_ROUTER,
            width: selected ? LINK_WIDTH_SELECTED : LINK_WIDTH,
        };
    }

    messageStyle(kind) {
        switch (kind) {
            case 'broadcast':
                return {color: COLOR_BROADCAST_MESSAGE, size: BROADCAST_MESSAGE_BEGIN_RADIUS};
            case 'ack':
                return {color: COLOR_ACK_MESSAGE, size: MESSAGE_SIZE};
            default:
                return {color: COLOR_UNICAST_MESSAGE, size: MESSAGE_SIZE};
        }
    }
}
