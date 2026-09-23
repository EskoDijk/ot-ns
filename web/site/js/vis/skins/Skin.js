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
// A skin defines the visual style of the network visualization: how nodes, their partition
// indicator, links and messages are drawn. Everything else (state tracking, gRPC handling,
// dragging, hit testing, windows) is skin-independent. See ./index.js for the registry of
// available skins.
//
// To add a skin: create skins/<name>.js with a class that extends Skin (or an existing skin)
// and overrides the hooks below, register it in skins/index.js, and on the Go side add the name
// to types.VisualizationSkins and a preset using it to types.VisualizationSkinPresets, so that
// the CLI command 'cv skin <preset>' can select it.

import * as PIXI from "pixi.js";

/**
 * Base class of all skins. The drawing hooks take a `state` object that describes a node:
 * {type: string, role: OtDeviceRole, nodeMode: NodeMode, failed: boolean}, where `type` is
 * the OTNS node type ('router', 'fed', 'sed', 'br', 'wifi', ...; see types/types.go).
 * Containers handed to the hooks are empty and centered at the node position (0,0).
 */
export default class Skin {
    /**
     * Build the node's shape into `container`, for the given node state.
     */
    buildNodeBody(container, state) {
        throw new Error("Skin.buildNodeBody() not implemented");
    }

    /**
     * Build the partition indicator into `container`, drawn on top of the node body.
     * @param color 24-bit RGB color derived from the node's partition ID
     */
    buildPartitionMark(container, state, color) {
        throw new Error("Skin.buildPartitionMark() not implemented");
    }

    /**
     * @returns {number} alpha (0..1) of the whole node body, including the partition mark.
     */
    nodeAlpha(state) {
        return 1.0;
    }

    /**
     * @returns {number} radius of the circular area around the node center that reacts to taps.
     */
    nodeHitRadius(state) {
        throw new Error("Skin.nodeHitRadius() not implemented");
    }

    /**
     * @returns {number} x and y offset of the node's text label, relative to the node center.
     */
    get labelOffset() {
        return 11;
    }

    /**
     * @returns {number} scale of the 'failed node' mark sprite that is drawn over a failed node.
     */
    get failedMarkScale() {
        return 0.5;
    }

    /**
     * Style of the marker of the selected node: a dashed box around the node and a circle
     * showing its radio range.
     * @returns {{boxColor: number, boxSize: number, boxAlpha: number, rangeFill: number,
     *            rangeFillAlpha: number, rangeStroke: number, rangeStrokeAlpha: number}}
     */
    selectionStyle() {
        throw new Error("Skin.selectionStyle() not implemented");
    }

    /**
     * Style of a link line between two nodes.
     * @param kind 'child': link between a parent and its child (from the child table or the
     *        child's parent); 'router': router-table link between two Routers; 'neighbor':
     *        router-table link of which at least one end is not a Router (an FTD child also
     *        reports router-table entries for the Routers it hears).
     * @param selected true if one end of the link is the selected node
     * @returns {{color: number, width: number}}
     */
    linkStyle(kind, selected) {
        throw new Error("Skin.linkStyle() not implemented");
    }

    /**
     * Style of an animated message.
     * @param kind 'broadcast', 'unicast' or 'ack'
     * @returns {{color: number, size: number}} for a broadcast message, `size` is the radius of
     *          the expanding circle when it starts; for the others it is the size of the sprite.
     */
    messageStyle(kind) {
        throw new Error("Skin.messageStyle() not implemented");
    }
}

/**
 * @returns {boolean} whether the OTNS node type is a Border Router type: the simulated 'br' or
 * the real-time 'otbr'. Both are drawn as a BR.
 */
export function isBorderRouter(nodeType) {
    return nodeType === 'br' || nodeType === 'otbr';
}

/**
 * Helper for skins: remove and destroy all children of `container`.
 */
export function clearContainer(container) {
    container.removeChildren().forEach(child => child.destroy());
}

/**
 * Helper for skins: create a centered sprite of `texture` (a white shape image of
 * `textureSize` pixels), tinted with `color` and scaled to the requested `size`.
 */
export function tintedSprite(texture, textureSize, size, color) {
    let sprite = new PIXI.Sprite(texture);
    sprite.anchor.set(0.5, 0.5);
    sprite.scale.set(size / textureSize);
    sprite.tint = color;
    return sprite;
}
