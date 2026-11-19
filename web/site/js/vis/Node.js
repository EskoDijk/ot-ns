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

import * as PIXI from "pixi.js";
import VObject from "./VObject";
import {NodeMode, OtDeviceRole} from '../proto/visualize_grpc_pb'
import {Visualizer} from "./PixiVisualizer";
import {Resources} from "./resources";
import {NODE_ID_INVALID, NODE_LABEL_FONT_FAMILY, NODE_LABEL_FONT_SIZE, POWER_DBM_INVALID,
        EXT_ADDR_INVALID} from "./consts";
import {Skin} from "./skins";
import {clearContainer} from "./skins/Skin";

const NODE_SELECTION_SCALE = 128;
const NODE_Z_SCALER = 2000;

let vis = Visualizer();

export default class Node extends VObject {
    constructor(nodeId, x, y, z, radioRange, nodeType) {
        super();

        this.id = nodeId;
        this.type = nodeType;
        this.threadVersion = 0;
        this.extAddr = EXT_ADDR_INVALID;
        this.radioRange = radioRange;
        this.nodeMode = new NodeMode([true, true, true, true]);
        this.rloc16 = 0xfffe;
        this.routerId = NODE_ID_INVALID;
        this.childId = NODE_ID_INVALID;
        this.parentId = NODE_ID_INVALID;
        this.role = OtDeviceRole.OT_DEVICE_ROLE_DISABLED;
        this.txPowerLast = POWER_DBM_INVALID;
        this.channelLast = -1;
        this.otVersion = "";
        this.otCommit = "";
        this._failed = false;
        this._parent = 0;
        this._partition = 0;
        this._children = {};
        this._neighbors = {};
        this._createTime = this.vis.curTime;
        // this._draggingOffset = null
        this._selected = false;

        this._root = new PIXI.Container();
        this.x = x;
        this.y = y;
        this.z = z;
        this.position.set(x, y);

        // the node body: its role/type dependent shape with a partition indicator on top. Both
        // are drawn by the active skin, see skins/Skin.js.
        let body = new PIXI.Container();
        this._shape = new PIXI.Container();
        body.addChild(this._shape);
        this._partitionMark = new PIXI.Container();
        this._partitionMark.visible = this.vis.visOptions.partitionId;
        body.addChild(this._partitionMark);
        this._root.addChild(body);
        this._body = body;

        let node = this;
        this._root.eventMode = 'static';
        this.setOnTouchStart((e) => {
            this.vis.setSelectedNode(node.id);
            e.stopPropagation();
        });
        this.setOnTap((e) => {
            e.stopPropagation();
        });

        this.setDraggable();

        let label = new PIXI.Text({text: "", style: {fontFamily: NODE_LABEL_FONT_FAMILY, fontSize: NODE_LABEL_FONT_SIZE, align: 'left'}});
        this._root.addChild(label);
        this.label = label;
        this._updateLabel();

        let failedMask = new PIXI.Sprite(Resources().FailedNodeMark.texture);
        failedMask.anchor.set(0.5, 0.5);
        failedMask.visible = false;
        this._root.addChild(failedMask);
        this._failedMask = failedMask;

        this.applySkin();
    }

    get failed() {
        return this._failed
    }

    set failed(v) {
        if (this._failed !== v) {
            this._failed = v;
            this._failedMask.visible = this._failed;
            this._redraw();
        }
    }

    get parent() {
        return this._parent
    }

    set parent(v) {
        this._parent = v
    }

    get partition() {
        return this._partition
    }

    set partition(v) {
        if (v !== this._partition) {
            this._partition = v;
            this._redrawPartitionMark();
        }
    }

    getActionContext() {
        return "node"
    }

    /**
     * @returns the node state that the skin uses to draw it, see skins/Skin.js.
     */
    _skinState() {
        return {type: this.type, role: this.role, nodeMode: this.nodeMode, failed: this.failed};
    }

    /**
     * (Re)build all skin-dependent parts of the node with the active skin: called after a skin
     * change and after a change of role, mode or failed state.
     */
    applySkin() {
        const skin = Skin();
        this._redraw();
        this.label.position.set(skin.labelOffset, skin.labelOffset);
        this._failedMask.scale.set(skin.failedMarkScale);
        if (this._selected) {
            this.onUnselected();
            this.onSelected();
        }
    }

    /**
     * Redraw the node shape after a change of skin, role, mode or failed state. This includes the
     * hit area, since a skin's nodeHitRadius() may depend on the node state.
     */
    _redraw() {
        const state = this._skinState();
        clearContainer(this._shape);
        Skin().buildNodeBody(this._shape, state);
        this._body.alpha = Skin().nodeAlpha(state);
        this._redrawPartitionMark();
        this._updateSize();
    }

    _redrawPartitionMark() {
        clearContainer(this._partitionMark);
        Skin().buildPartitionMark(this._partitionMark, this._skinState(), this.vis.getPartitionColor(this._partition));
    }

    setPartitionVisible(visible) {
        this._partitionMark.visible = visible;
    }

    setPosition(x, y, z) {
        let zChanged = z != this.z;
        this.x = x;
        this.y = y;
        this.z = z;
        if (!this.isDragging()) {
            this.position.set(x, y)
        }
        if (zChanged) {
            this._updateSize(); // higher (z coord) nodes appear larger.
        }
    }

    _updateLabel() {
        let rloc16 = ('0000' + this.rloc16.toString(16).toUpperCase()).slice(-4);
        this.label.text = this.id.toString() + "|" + rloc16
    }

    _updateSize() {
        let heightScale = 1.0 + this.z / NODE_Z_SCALER;
        this._body.scale.set(heightScale);
        this._root.hitArea = new PIXI.Circle(0, 0, Skin().nodeHitRadius(this._skinState()) * heightScale);
    }

    setRloc16(rloc16) {
        this.rloc16 = rloc16;
        this.routerId = rloc16 >> 10;
        this.childId = rloc16 & 0x01ff;
        this._updateLabel()
    }

    setRole(role) {
        if (role != this.role) {
            this.role = role;
            this._redraw();
        }
        if (role == OtDeviceRole.OT_DEVICE_ROLE_DISABLED || role == OtDeviceRole.OT_DEVICE_ROLE_DETACHED) {
            this._parent = NODE_ID_INVALID;
            this.parentId = NODE_ID_INVALID;
            this.childId = NODE_ID_INVALID;
            this.routerId = NODE_ID_INVALID;
        }
    }

    setMode(mode) {
        if (mode != this.nodeMode) {
            this.nodeMode = mode;
            this._redraw();
        }
    }

    setThreadVersion(version) {
        this.threadVersion = version;
    }

    setOTVersion(version, commit) {
        this.otVersion = version;
        this.otCommit = commit;
    }

    addRouterTable(extaddr) {
        this._neighbors[extaddr] = 1
    }

    removeRouterTable(extaddr) {
        delete this._neighbors[extaddr]
    }

    addChildTable(extaddr) {
        this._children[extaddr] = 1
    }

    removeChildTable(extaddr) {
        delete this._children[extaddr]
    }

    onDraggingTimer() {
        let pos = this.position;
        this.vis.ctrlMoveNodeTo(this.id, pos.x, pos.y, (err, resp) => {
        })
    }

    onDraggingDone() {
        let pos = this.position;
        this.vis.ctrlMoveNodeTo(this.id, pos.x, pos.y, (err, resp) => {
            if (err !== null) {
                this.position.set(this.x, this.y)
            }
        })
    }

    update(dt) {
        super.update(dt);
        // this._updateDragging(dt)
    }

    onSelected() {
        this._selected = true;
        if (!this._selbox) {
            const style = Skin().selectionStyle();
            let selbox = new PIXI.Sprite(Resources().WhiteRoundedDashedSquare128.texture);
            selbox.tint = style.boxColor;
            selbox.alpha = style.boxAlpha;
            selbox.scale.set(style.boxSize / NODE_SELECTION_SCALE);
            selbox.anchor.set(0.5, 0.5);
            this.root.addChildAt(selbox, 0);
            this._selbox = selbox;

            const rangeCircleSize = this.radioRange;
            let rangeCircle = new PIXI.Graphics();
            rangeCircle.circle(0, 0, rangeCircleSize);
            rangeCircle.fill({color: style.rangeFill, alpha: style.rangeFillAlpha});
            rangeCircle.stroke({width: 1, color: style.rangeStroke, alpha: style.rangeStrokeAlpha});
            this.root.addChildAt(rangeCircle, 0);
            this._rangeCircle = rangeCircle;
        }
    }

    onUnselected() {
        this._selected = false;
        if (this._selbox) {
            this._selbox.destroy();
            delete this._selbox;

            this._rangeCircle.destroy();
            delete this._rangeCircle;
        }
    }
}
