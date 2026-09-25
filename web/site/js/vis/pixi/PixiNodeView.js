// Copyright (c) 2020-2026, The OTNS Authors.
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
// PixiNodeView is the 2D drawing of one node: its skin-drawn shape with the partition mark on
// top, the label, the 'failed' mark, and when selected the selection box and radio range circle.
// It also handles taps (select) and drags (move) on the node.

import * as PIXI from "pixi.js";
import VObject from "../VObject";
import {Resources} from "../resources";
import {NODE_LABEL_FONT_FAMILY, NODE_LABEL_FONT_SIZE} from "../consts";
import {Skin} from "../skins";
import {clearContainer} from "../skins/Skin";

const NODE_SELECTION_SCALE = 128;
const NODE_Z_SCALER = 2000;

export default class PixiNodeView extends VObject {
    /**
     * @param state {NodeState} the node's state, owned by the visualizer
     */
    constructor(state) {
        super();
        this.state = state;
        this._selected = false;

        this._root = new PIXI.Container();
        this.position.set(state.x, state.y);

        // the node body: its role/type dependent shape with a partition indicator on top. Both
        // are drawn by the active skin, see skins/Skin.js.
        let body = new PIXI.Container();
        this._shape = new PIXI.Container();
        body.addChild(this._shape);
        this._partitionMark = new PIXI.Container();
        body.addChild(this._partitionMark);
        this._root.addChild(body);
        this._body = body;

        this._root.eventMode = 'static';
        this.setOnTouchStart((e) => {
            this.vis.setSelectedNode(state.id);
            e.stopPropagation();
        });
        this.setOnTap((e) => {
            e.stopPropagation();
        });
        this.setDraggable();

        this.label = new PIXI.Text({text: "", style: {fontFamily: NODE_LABEL_FONT_FAMILY, fontSize: NODE_LABEL_FONT_SIZE, align: 'left'}});
        this._root.addChild(this.label);

        let failedMask = new PIXI.Sprite(Resources().FailedNodeMark.texture);
        failedMask.anchor.set(0.5, 0.5);
        failedMask.visible = false;
        this._root.addChild(failedMask);
        this._failedMask = failedMask;

        this.applySkin();
    }

    get id() {
        return this.state.id;
    }

    /**
     * (Re)build all skin-dependent parts of the node with the active skin.
     */
    applySkin() {
        const skin = Skin();
        this.redraw();
        this.label.position.set(skin.labelOffset, skin.labelOffset);
        this._failedMask.scale.set(skin.failedMarkScale);
        if (this._selected) {
            this._destroySelection();
            this._createSelection();
        }
    }

    /**
     * Redraw the node after a change of its state (role, mode, failed, partition, RLOC16). This
     * includes the hit area, since a skin's nodeHitRadius() may depend on the node state.
     */
    redraw() {
        const state = this.state.skinState();
        clearContainer(this._shape);
        Skin().buildNodeBody(this._shape, state);
        this._body.alpha = Skin().nodeAlpha(state);
        this._redrawPartitionMark();
        this._failedMask.visible = this.state.failed;
        const rloc16 = ('0000' + this.state.rloc16.toString(16).toUpperCase()).slice(-4);
        this.label.text = this.state.id.toString() + "|" + rloc16;
        this._updateSize();
    }

    _redrawPartitionMark() {
        clearContainer(this._partitionMark);
        Skin().buildPartitionMark(this._partitionMark, this.state.skinState(), this.vis.getPartitionColor(this.state.partition));
    }

    setPartitionVisible(visible) {
        this._partitionMark.visible = visible;
    }

    /**
     * The state's position changed. While the user drags the node, the drag position is kept.
     */
    onPositionChanged() {
        if (!this.isDragging()) {
            this.position.set(this.state.x, this.state.y);
        }
        this._updateSize(); // higher (z coord) nodes appear larger.
    }

    _updateSize() {
        let heightScale = 1.0 + this.state.z / NODE_Z_SCALER;
        this._body.scale.set(heightScale);
        this._root.hitArea = new PIXI.Circle(0, 0, Skin().nodeHitRadius(this.state.skinState()) * heightScale);
    }

    onDraggingTimer() {
        let pos = this.position;
        this.vis.ctrlMoveNodeTo(this.state.id, pos.x, pos.y, (err, resp) => {
        })
    }

    onDraggingDone() {
        let pos = this.position;
        this.vis.ctrlMoveNodeTo(this.state.id, pos.x, pos.y, (err, resp) => {
            if (err !== null) {
                this.position.set(this.state.x, this.state.y)
            }
        })
    }

    setSelected(selected) {
        if (selected === this._selected) {
            return;
        }
        this._selected = selected;
        if (selected) {
            this._createSelection();
        } else {
            this._destroySelection();
        }
    }

    _createSelection() {
        const style = Skin().selectionStyle();
        let selbox = new PIXI.Sprite(Resources().WhiteRoundedDashedSquare128.texture);
        selbox.tint = style.boxColor;
        selbox.alpha = style.boxAlpha;
        selbox.scale.set(style.boxSize / NODE_SELECTION_SCALE);
        selbox.anchor.set(0.5, 0.5);
        this._root.addChildAt(selbox, 0);
        this._selbox = selbox;

        let rangeCircle = new PIXI.Graphics();
        rangeCircle.circle(0, 0, this.state.radioRange);
        rangeCircle.fill({color: style.rangeFill, alpha: style.rangeFillAlpha});
        rangeCircle.stroke({width: 1, color: style.rangeStroke, alpha: style.rangeStrokeAlpha});
        this._root.addChildAt(rangeCircle, 0);
        this._rangeCircle = rangeCircle;
    }

    _destroySelection() {
        if (this._selbox) {
            this._selbox.destroy();
            delete this._selbox;
            this._rangeCircle.destroy();
            delete this._rangeCircle;
        }
    }
}
