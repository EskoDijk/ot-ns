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
// PixiFieldRenderer draws the 2D field with Pixi, styled by the active skin (skins/Skin.js).
// See FieldRenderer.js for the interface.

import * as PIXI from "pixi.js";
import FieldRenderer from "../FieldRenderer";
import PixiNodeView from "./PixiNodeView";
import {AckMessage, BroadcastMessage, UnicastMessage} from "./PixiMessages";
import {Skin} from "../skins";

export default class PixiFieldRenderer extends FieldRenderer {
    constructor(vis) {
        super(vis);
        this.kind = 'pixi';
        this._views = {};        // node ID -> PixiNodeView
        this._messages = {};     // message ID -> message view
        this._partitionVisible = vis.visOptions.partitionId;

        this._backLayer = new PIXI.Container();
        this._linksStage = new PIXI.Container();
        this._backLayer.addChild(this._linksStage);
        this._broadcastMessagesStage = new PIXI.Container();
        this._backLayer.addChild(this._broadcastMessagesStage);

        this._frontLayer = new PIXI.Container();
        this._nodesStage = new PIXI.Container();
        this._frontLayer.addChild(this._nodesStage);
        this._unicastMessagesStage = new PIXI.Container();
        this._frontLayer.addChild(this._unicastMessagesStage);
    }

    get backLayer() {
        return this._backLayer;
    }

    get frontLayer() {
        return this._frontLayer;
    }

    addNode(state) {
        let view = new PixiNodeView(state);
        view.setPartitionVisible(this._partitionVisible);
        this._views[state.id] = view;
        this._nodesStage.addChild(view.root);
    }

    removeNode(state) {
        let view = this._views[state.id];
        if (view) {
            delete this._views[state.id];
            view.destroy();
        }
    }

    updateNode(state) {
        this._views[state.id].redraw();
    }

    moveNode(state) {
        this._views[state.id].onPositionChanged();
    }

    setSelectedNode(state) {
        for (let id in this._views) {
            this._views[id].setSelected(state !== null && Number(id) === state.id);
        }
    }

    setPartitionVisible(visible) {
        this._partitionVisible = visible;
        for (let id in this._views) {
            this._views[id].setPartitionVisible(visible);
        }
    }

    applySkin() {
        for (let id in this._views) {
            this._views[id].applySkin();
        }
    }

    drawLinks(links) {
        this._linksStage.removeChildren().forEach(child => child.destroy());
        const graphics = new PIXI.Graphics();

        // Pixi v8 strokes the whole current path with a single style, so group segments by
        // (color, width) and stroke each group as its own path. Endpoints are the views'
        // positions, so that links follow a dragged node.
        const skin = Skin();
        const groups = {};
        for (let link of links) {
            const from = this._views[link.from.id];
            const to = this._views[link.to.id];
            if (!from || !to) {
                continue;
            }
            const style = skin.linkStyle(link.kind, link.selected);
            const key = style.color + ":" + style.width;
            if (!(key in groups)) {
                groups[key] = {style: style, segments: []};
            }
            groups[key].segments.push([from.position, to.position]);
        }

        for (let key in groups) {
            const {style, segments} = groups[key];
            graphics.beginPath();
            for (let [from, to] of segments) {
                graphics.moveTo(from.x, from.y).lineTo(to.x, to.y);
            }
            graphics.stroke({width: style.width, color: style.color, alpha: 1});
        }
        this._linksStage.addChild(graphics);
    }

    showBroadcast(src, mvInfo) {
        this._addMessage(new BroadcastMessage(this._views[src.id], mvInfo), this._broadcastMessagesStage);
    }

    showUnicast(src, dst, mvInfo) {
        const dstView = dst !== null ? this._views[dst.id] : null;
        this._addMessage(new UnicastMessage(this._views[src.id], dstView, mvInfo), this._unicastMessagesStage);
    }

    showAck(src, mvInfo) {
        this._addMessage(new AckMessage(this._views[src.id], mvInfo), this._unicastMessagesStage);
    }

    _addMessage(msg, stage) {
        stage.addChild(msg.root);
        this._messages[msg.id] = msg;
    }

    update(dt) {
        for (let id in this._views) {
            this._views[id].update(dt);
        }
        for (let id in this._messages) {
            let msg = this._messages[id];
            if (!msg.update(dt)) {
                delete this._messages[id];
                msg.destroy();
            }
        }
    }

    destroy() {
        this._views = {};
        this._messages = {};
        this._backLayer.destroy({children: true});
        this._frontLayer.destroy({children: true});
    }
}
