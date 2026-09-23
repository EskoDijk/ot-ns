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
// The 'thread' skin: nodes and links are drawn following the common Thread network
// diagram conventions:
//   - Router: solid orange pentagon. Leader: solid grey pentagon.
//   - End Device: circle with white fill; orange outline if router-capable (REED),
//     grey outline if not (FED/MED/SED/SSED). FTDs (REED/FED) get a thick outline, MTDs a
//     thin one; sleepy MTDs get a dashed outline: 6 dashes = SSED, 8 dashes = SED.
//   - Border Router (BR): solid black square. A Leader BR gets a grey outline; a BR that
//     is not (yet) a Router is drawn with white fill and black outline.
//   - Wi-Fi interferer node: solid grey circle.
//   - Detached, disabled and failed nodes are drawn semi-transparent.
//   - Links: orange between two Routers, thinner dark grey between a Router and an End Device.
//   - The partition of a node, if enabled to show it, is shown as a colored dot in its center.

import * as PIXI from "pixi.js";
import {OtDeviceRole} from '../../proto/visualize_grpc_pb'
import Skin, {isBorderRouter} from "./Skin";

const COLOR_THREAD_ORANGE = 0xfd4f27;   // Routers, router-capable End Devices, Thread links
const COLOR_THREAD_GREY = 0x8499b0;     // Leader, End Devices that are not router-capable
const COLOR_BR_BLACK = 0x363636;        // Border Routers
const COLOR_NODE_FILL = 0xffffff;       // fill of End Device shapes
const COLOR_NODE_SELECTION = 0x363636;
const COLOR_LINK_ROUTER = COLOR_THREAD_ORANGE; // router-to-router links
const COLOR_LINK_PARENT_CHILD = 0x555555;      // Router to End Device (child) links
const COLOR_UNICAST_MESSAGE = 0x1565c0;
const COLOR_BROADCAST_MESSAGE = 0x1565c0;
const COLOR_ACK_MESSAGE = 0xaee571;
const LINK_WIDTH_PARENT_CHILD = 1;
const LINK_WIDTH_ROUTER = 2;
const LINK_WIDTH_SELECTED_EXTRA = 2;           // added to links of the selected node

const CIRCULAR_SHAPE_RADIUS = 35;
const PENTAGON_SHAPE_RADIUS = 38;
const SQUARE_SHAPE_RADIUS = 33; // half of the square's side
const NODE_MAX_RADIUS = PENTAGON_SHAPE_RADIUS;
const OUTLINE_WIDTH_MTD = 4;
const OUTLINE_WIDTH_FTD = 6;
const OUTLINE_WIDTH_LEADER_BR = 7;
const DASH_FRACTION = 0.6; // drawn part of each dash period of a dashed outline
const ALPHA_DETACHED = 0.5;
const ALPHA_DISABLED = 0.3;
const PARTITION_DOT_RADIUS = 10;
const NODE_LABEL_OFFSET = 26; // label starts just outside the node shape (bottom-right)
const FAILED_MARK_SCALE = 0.85;
const NODE_SELECTION_BOX_SIZE = 105;
const BROADCAST_MESSAGE_BEGIN_RADIUS = 40;
const MESSAGE_SIZE = 16;

// Node types (see types/types.go) that can become a Router, resp. cannot. Any other node
// type (e.g. an externally started node) is classified by its Thread mode: FTD -> router-capable.
const ROUTER_CAPABLE_TYPES = ['router', 'reed', 'ftd', 'br', 'otbr', 'matter'];
const NOT_ROUTER_CAPABLE_TYPES = ['fed', 'wifi', 'med', 'mtd', 'sed', 'ssed'];

function isRouterCapable(nodeType, nodeMode) {
    if (ROUTER_CAPABLE_TYPES.includes(nodeType)) {
        return true;
    }
    if (NOT_ROUTER_CAPABLE_TYPES.includes(nodeType)) {
        return false;
    }
    return nodeMode.getFullThreadDevice();
}

/**
 * Determine the visual style of a node from its type ('router', 'fed', 'br', ...), OtDeviceRole,
 * NodeMode and failed state.
 * @returns {{shape: string, radius: number, fill: number, outline: (number|null), outlineWidth: number,
 *            dashes: number, alpha: number}}
 */
function getNodeVisualStyle(nodeType, role, nodeMode, failed) {
    const isLeader = role === OtDeviceRole.OT_DEVICE_ROLE_LEADER;
    const isRouterRole = isLeader || role === OtDeviceRole.OT_DEVICE_ROLE_ROUTER;
    let style = {
        shape: 'circle',
        radius: CIRCULAR_SHAPE_RADIUS,
        fill: COLOR_NODE_FILL,
        outline: COLOR_THREAD_GREY,
        outlineWidth: OUTLINE_WIDTH_FTD,
        dashes: 0,
        alpha: 1.0,
    };

    if (isBorderRouter(nodeType)) {
        style.shape = 'square';
        style.radius = SQUARE_SHAPE_RADIUS;
        style.outline = COLOR_BR_BLACK;
        if (isRouterRole) {
            style.fill = COLOR_BR_BLACK;
            style.outline = isLeader ? COLOR_THREAD_GREY : null;
            style.outlineWidth = OUTLINE_WIDTH_LEADER_BR;
        }
    } else if (nodeType === 'wifi') {
        style.fill = COLOR_THREAD_GREY;
        style.outline = null;
    } else if (isRouterRole) {
        style.shape = 'pentagon';
        style.radius = PENTAGON_SHAPE_RADIUS;
        style.fill = isLeader ? COLOR_THREAD_GREY : COLOR_THREAD_ORANGE;
        style.outline = null;
    } else {
        style.outline = isRouterCapable(nodeType, nodeMode) ? COLOR_THREAD_ORANGE : COLOR_THREAD_GREY;
        if (!nodeMode.getFullThreadDevice()) {
            style.outlineWidth = OUTLINE_WIDTH_MTD;
            if (!nodeMode.getRxOnWhenIdle()) {
                style.dashes = nodeType === 'ssed' ? 6 : 8;
            }
        }
    }

    // a Wi-Fi node never joins the Thread network, so it is not shown as detached/disabled.
    const useRole = nodeType !== 'wifi';
    if (failed || (useRole && role === OtDeviceRole.OT_DEVICE_ROLE_DETACHED)) {
        style.alpha = ALPHA_DETACHED;
    } else if (useRole && role === OtDeviceRole.OT_DEVICE_ROLE_DISABLED) {
        style.alpha = ALPHA_DISABLED;
    }
    return style;
}

/**
 * Draw the node shape described by `style` (see getNodeVisualStyle) into `graphics`, centered
 * at (0,0). The caller is responsible for applying style.alpha.
 */
function drawNodeShape(graphics, style) {
    graphics.clear();
    const hasOutline = style.outline !== null;
    // the stroke is centered on the path, so inset the path to keep the outer size at style.radius.
    const r = hasOutline ? style.radius - style.outlineWidth / 2 : style.radius;
    switch (style.shape) {
        case 'pentagon':
            graphics.regularPoly(0, 0, r, 5);
            break;
        case 'square':
            graphics.rect(-r, -r, 2 * r, 2 * r);
            break;
        default:
            graphics.circle(0, 0, r);
    }
    graphics.fill({color: style.fill});
    if (!hasOutline) {
        return;
    }
    if (style.dashes > 0) {
        // dashed outline: stroke separate arcs instead of the shape's own path.
        const period = 2 * Math.PI / style.dashes;
        for (let i = 0; i < style.dashes; i++) {
            const a0 = -Math.PI / 2 + i * period;
            graphics.moveTo(r * Math.cos(a0), r * Math.sin(a0));
            graphics.arc(0, 0, r, a0, a0 + period * DASH_FRACTION);
        }
    }
    graphics.stroke({width: style.outlineWidth, color: style.outline});
}

export default class ThreadSkin extends Skin {
    buildNodeBody(container, state) {
        let graphics = new PIXI.Graphics();
        drawNodeShape(graphics, getNodeVisualStyle(state.type, state.role, state.nodeMode, state.failed));
        container.addChild(graphics);
    }

    buildPartitionMark(container, state, color) {
        let dot = new PIXI.Graphics();
        dot.circle(0, 0, PARTITION_DOT_RADIUS);
        dot.fill({color: color});
        container.addChild(dot);
    }

    nodeAlpha(state) {
        return getNodeVisualStyle(state.type, state.role, state.nodeMode, state.failed).alpha;
    }

    nodeHitRadius(state) {
        return NODE_MAX_RADIUS;
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
            rangeFill: COLOR_THREAD_GREY, rangeFillAlpha: 0.12,
            rangeStroke: COLOR_THREAD_GREY, rangeStrokeAlpha: 0.7,
        };
    }

    linkStyle(kind, selected) {
        const extra = selected ? LINK_WIDTH_SELECTED_EXTRA : 0;
        if (kind === 'router') {
            return {color: COLOR_LINK_ROUTER, width: LINK_WIDTH_ROUTER + extra};
        }
        return {color: COLOR_LINK_PARENT_CHILD, width: LINK_WIDTH_PARENT_CHILD + extra};
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
