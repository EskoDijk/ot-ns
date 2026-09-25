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
// A FieldRenderer draws the "field" of the visualization: the nodes, the links between them, the
// animated messages and the marker of the selected node. It is told about state changes by
// PixiVisualizer, which owns the node states (NodeState) and everything else: gRPC handling, the
// selection, the action bar, the log and node windows, and keyboard handling.
//
// Renderers: pixi/PixiFieldRenderer.js draws the 2D field with the active Skin (skins/Skin.js).
// A renderer keeps its own per-node view objects, keyed by node ID, and is the only place where
// node positions are turned into screen coordinates.

export default class FieldRenderer {
    /**
     * @param vis the PixiVisualizer that owns the node states
     */
    constructor(vis) {
        this.vis = vis;
    }

    /**
     * @returns {PIXI.Container} layer drawn behind the log window: links and broadcast messages.
     */
    get backLayer() {
        throw new Error("FieldRenderer.backLayer not implemented");
    }

    /**
     * @returns {PIXI.Container} layer drawn in front of the log window: nodes and unicast messages.
     */
    get frontLayer() {
        throw new Error("FieldRenderer.frontLayer not implemented");
    }

    /**
     * A node was added. Its view must react to taps by calling vis.setSelectedNode(state.id) and
     * to drags by calling vis.ctrlMoveNodeTo().
     * @param state {NodeState}
     */
    addNode(state) {
        throw new Error("FieldRenderer.addNode() not implemented");
    }

    /**
     * A node was deleted.
     * @param state {NodeState} the state, already removed from vis.nodes
     */
    removeNode(state) {
        throw new Error("FieldRenderer.removeNode() not implemented");
    }

    /**
     * The appearance-relevant state of a node changed: role, mode, failed, partition or RLOC16.
     * @param state {NodeState}
     */
    updateNode(state) {
        throw new Error("FieldRenderer.updateNode() not implemented");
    }

    /**
     * The simulator reported a new position of a node. A view that is being dragged by the user
     * keeps its drag position until the drag ends.
     * @param state {NodeState}
     */
    moveNode(state) {
        throw new Error("FieldRenderer.moveNode() not implemented");
    }

    /**
     * Mark `state` as the selected node (its marker and radio range are shown), unmark all others.
     * @param state {NodeState|null} null if no node is selected
     */
    setSelectedNode(state) {
        throw new Error("FieldRenderer.setSelectedNode() not implemented");
    }

    /**
     * Show or hide the partition marks of all nodes ('cv pid' option).
     */
    setPartitionVisible(visible) {
        throw new Error("FieldRenderer.setPartitionVisible() not implemented");
    }

    /**
     * The active skin changed (see skins/index.js): rebuild all skin-dependent objects.
     */
    applySkin() {
        throw new Error("FieldRenderer.applySkin() not implemented");
    }

    /**
     * Draw the links between nodes; called every frame since nodes may be dragged.
     * @param links {Array<{kind: string, selected: boolean, from: NodeState, to: NodeState}>}
     *        `kind` is 'child', 'router' or 'neighbor' as defined by Skin.linkStyle(); `selected`
     *        is true if one end of the link is the selected node.
     */
    drawLinks(links) {
        throw new Error("FieldRenderer.drawLinks() not implemented");
    }

    /**
     * Animate a broadcast message sent by `src`.
     * @param src {NodeState}
     * @param mvInfo MsgVisualizeInfo of the message
     */
    showBroadcast(src, mvInfo) {
        throw new Error("FieldRenderer.showBroadcast() not implemented");
    }

    /**
     * Animate a unicast message from `src` to `dst`.
     * @param src {NodeState}
     * @param dst {NodeState|null} null if the destination is not a known node
     * @param mvInfo MsgVisualizeInfo of the message
     */
    showUnicast(src, dst, mvInfo) {
        throw new Error("FieldRenderer.showUnicast() not implemented");
    }

    /**
     * Animate an ACK frame sent by `src`.
     * @param src {NodeState}
     * @param mvInfo MsgVisualizeInfo of the message
     */
    showAck(src, mvInfo) {
        throw new Error("FieldRenderer.showAck() not implemented");
    }

    /**
     * Advance animations and drag timers by `dt` seconds of real time.
     */
    update(dt) {
        throw new Error("FieldRenderer.update() not implemented");
    }

    /**
     * Release all drawing objects; the renderer is not used afterwards.
     */
    destroy() {
        throw new Error("FieldRenderer.destroy() not implemented");
    }
}
