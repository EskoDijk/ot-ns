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
// The 2D animations of messages: an expanding circle for a broadcast, a moving sprite for a
// unicast message or an ACK. Sizes and colors come from the active skin (skins/Skin.js); the
// timing from Lifetime.js. A message follows its (possibly dragged) source and destination views.

import * as PIXI from "pixi.js";
import VObject from "../VObject";
import Lifetime from "../Lifetime";
import {Resources} from "../resources";
import {Skin} from "../skins";

const BROADCAST_MESSAGE_SCALE = 128;
const UNICAST_MESSAGE_SCALE = 64;

let nextMessageId = 1;

class MessageView extends VObject {
    constructor(mvInfo) {
        super();
        this.id = nextMessageId;
        nextMessageId += 1;
        this.mvInfo = mvInfo;
        this.lifetime = new Lifetime(this.vis);
        this.lifetime.configure(mvInfo);
    }

    _createSprite(texture, size, textureSize, color) {
        let sprite = new PIXI.Sprite(texture);
        sprite.tint = color;
        sprite.scale.set(size / textureSize);
        sprite.anchor.set(0.5, 0.5);
        this._root = sprite;
        return sprite;
    }

    /**
     * @returns {boolean} false once the message's lifetime is over and it must be deleted.
     */
    update(dt) {
        super.update(dt);
        this.lifetime.update(dt);
        return !this.lifetime.isOver();
    }
}

export class BroadcastMessage extends MessageView {
    /**
     * @param src {PixiNodeView} the sending node
     */
    constructor(src, mvInfo) {
        super(mvInfo);
        this.src = src;
        this.style = Skin().messageStyle('broadcast');
        this._targetRadius = src.state.radioRange;
        let sprite = this._createSprite(Resources().WhiteDashed8Circle128.texture, this.style.size * 2, BROADCAST_MESSAGE_SCALE, this.style.color);
        sprite.alpha = 0.1;
        sprite.position.copyFrom(src.position);
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        let beginRadius = this.style.size;
        let radius = beginRadius + (this._targetRadius - beginRadius) * Math.pow(this.lifetime.progress(), 0.5);
        this._root.scale.set(radius * 2 / BROADCAST_MESSAGE_SCALE);
        this._root.position.copyFrom(this.src.position); // track the (possibly moving) source
        return true;
    }
}

export class UnicastMessage extends MessageView {
    /**
     * @param src {PixiNodeView} the sending node
     * @param dst {PixiNodeView|null} the destination node; null if unknown, the message then
     *        moves a fixed distance away from the source.
     */
    constructor(src, dst, mvInfo) {
        super(mvInfo);
        this.src = src;
        this.dst = dst;
        this.dstOffset = new PIXI.Point(0, 200);
        this.style = Skin().messageStyle('unicast');
        let sprite = this._createSprite(Resources().WhiteSolidHexagon64.texture, this.style.size, UNICAST_MESSAGE_SCALE, this.style.color);
        sprite.position.copyFrom(src.position);
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        let dstx, dsty;
        if (this.dst !== null) { // track the (possibly moving) destination
            dstx = this.dst.position.x;
            dsty = this.dst.position.y;
        } else {
            dstx = this.src.position.x + this.dstOffset.x;
            dsty = this.src.position.y + this.dstOffset.y;
        }
        let r = this.lifetime.progress();
        this._root.position.set(this.src.position.x + r * (dstx - this.src.position.x),
            this.src.position.y + r * (dsty - this.src.position.y));
        return true;
    }
}

export class AckMessage extends MessageView {
    /**
     * @param src {PixiNodeView} the sending node
     */
    constructor(src, mvInfo) {
        super(mvInfo);
        this.src = src;
        this.dstOffset = new PIXI.Point(0, 50);
        this.style = Skin().messageStyle('ack');
        let sprite = this._createSprite(Resources().WhiteSolidTriangle64.texture, this.style.size, UNICAST_MESSAGE_SCALE, this.style.color);
        sprite.position.copyFrom(src.position);
    }

    update(dt) {
        if (!super.update(dt)) {
            return false;
        }
        let r = this.lifetime.progress();
        this._root.position.set(this.src.position.x + r * this.dstOffset.x, this.src.position.y + r * this.dstOffset.y);
        return true;
    }
}
