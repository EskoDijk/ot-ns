// Copyright (c) 2020-2024, The OTNS Authors.
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
        EXT_ADDR_INVALID, COLOR_NODE_SELECTION, COLOR_THREAD_GREY} from "./consts";
import {NODE_MAX_RADIUS, getNodeVisualStyle, drawNodeShape} from "./nodeStyle";

const NODE_SELECTION_SCALE = 128;
const NODE_SELECTION_BOX_SIZE = 105;
const NODE_Z_SCALER = 2000;
const NODE_LABEL_OFFSET = 26; // label starts just outside the node shape (bottom-right)
const PARTITION_DOT_RADIUS = 10;
const FAILED_MARK_SCALE = 0.85;

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

        // the node body: its role/type dependent shape with a partition indicator dot on top.
        let body = new PIXI.Container();
        this._shape = new PIXI.Graphics();
        body.addChild(this._shape);
        this._partitionDot = new PIXI.Graphics();
        this._partitionDot.visible = this.vis.visOptions.partitionId;
        body.addChild(this._partitionDot);
        this._root.addChild(body);
        this._body = body;
        this._redraw();

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

        this._updateSize();

        let label = new PIXI.Text({text: "", style: {fontFamily: NODE_LABEL_FONT_FAMILY, fontSize: NODE_LABEL_FONT_SIZE, align: 'left'}});
        label.position.set(NODE_LABEL_OFFSET, NODE_LABEL_OFFSET);
        this._root.addChild(label);
        this.label = label;
        this._updateLabel();

        let failedMask = new PIXI.Sprite(Resources().FailedNodeMark.texture);
        failedMask.anchor.set(0.5, 0.5);
        failedMask.scale.set(FAILED_MARK_SCALE, FAILED_MARK_SCALE);
        failedMask.visible = false;
        this._root.addChild(failedMask);
        this._failedMask = failedMask
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
            this._redrawPartitionDot();
        }
    }

    getActionContext() {
        return "node"
    }

    /**
     * Redraw the node shape after a change of role, mode or failed state. See nodeStyle.js
     * for the visual style rules.
     */
    _redraw() {
        let style = getNodeVisualStyle(this.type, this.role, this.nodeMode, this.failed);
        drawNodeShape(this._shape, style);
        this._body.alpha = style.alpha;
        this._redrawPartitionDot();
    }

    _redrawPartitionDot() {
        this._partitionDot.clear();
        this._partitionDot.circle(0, 0, PARTITION_DOT_RADIUS);
        this._partitionDot.fill({color: this.vis.getPartitionColor(this._partition)});
    }

    setPartitionVisible(visible) {
        this._partitionDot.visible = visible;
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
        this._root.hitArea = new PIXI.Circle(0, 0, NODE_MAX_RADIUS * heightScale);
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
            let selbox = new PIXI.Sprite(Resources().WhiteRoundedDashedSquare128.texture);
            selbox.tint = COLOR_NODE_SELECTION;
            selbox.alpha = 0.7;
            selbox.scale.set(NODE_SELECTION_BOX_SIZE / NODE_SELECTION_SCALE, NODE_SELECTION_BOX_SIZE / NODE_SELECTION_SCALE);
            selbox.anchor.set(0.5, 0.5);
            this.root.addChildAt(selbox, 0);
            this._selbox = selbox;

            const rangeCircleSize = this.radioRange;
            let rangeCircle = new PIXI.Graphics();
            rangeCircle.circle(0, 0, rangeCircleSize);
            rangeCircle.fill({color: COLOR_THREAD_GREY, alpha: 0.12});
            rangeCircle.stroke({width: 1, color: COLOR_THREAD_GREY, alpha: 0.7});
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
