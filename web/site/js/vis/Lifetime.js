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
// Lifetime tracks the remaining display time of an animated message. A message either lives for
// its true transmission duration in simulated time (which is stretched or compressed by the
// simulation speed, and frozen while paused) or for a fixed real-time duration.

export default class Lifetime {
    /**
     * @param vis the visualizer, for the current simulation time and speed
     */
    constructor(vis) {
        this.vis = vis;
        this._lifetime = 0;
        this._lifetimeRemaining = 0;
        this._lifetimeRealMode = true;
    }

    /**
     * Configure the lifetime from the message's visualization info: its true send duration in
     * simulated time, or else a default real-time duration.
     * @param mvInfo MsgVisualizeInfo of the message
     * @param defaultRealDuration real-time duration in us, used when the true duration isn't shown
     */
    configure(mvInfo, defaultRealDuration = 700000) {
        if (mvInfo.getVisTrueDuration()) {
            this.setVirtual(mvInfo.getSendDurationUs());
        } else {
            this.setReal(defaultRealDuration);
        }
    }

    // set the lifetime to a virtual (simulated time) duration in us.
    setVirtual(dtSimUs) {
        this._startSimTime = this.vis.curTime;
        this._lastSimTime = this._startSimTime;
        this._lifetime = dtSimUs / 1000000;
        this._lifetimeRemaining = this._lifetime;
        this._lifetimeRealMode = false;
    }

    // set the lifetime to a real (clock) duration in us.
    setReal(dtRealUs) {
        this._lifetime = dtRealUs / 1000000;
        this._lifetimeRemaining = this._lifetime;
        this._lifetimeRealMode = true;
    }

    /**
     * Advance the lifetime by `dt` seconds of real time.
     */
    update(dt) {
        if (this._lifetimeRemaining > 0) {
            if (this._lifetimeRealMode) {
                this._lifetimeRemaining -= dt;
            } else {
                if (this.vis.isPaused() || this.vis.curTime > this._lastSimTime) {
                    this._lifetimeRemaining = this._lifetime - (this.vis.curTime - this._startSimTime) / 1000000;
                    this._lastSimTime = this.vis.curTime;
                } else {
                    this._lifetimeRemaining -= dt * this.vis.speed;
                }
            }
            if (this._lifetimeRemaining < 0) {
                this._lifetimeRemaining = 0;
            }
        }
    }

    /**
     * @returns {number} real-time equivalent of the remaining lifetime in seconds, or 0 if over.
     *          May be Infinity while the simulation is paused.
     */
    realRemaining() {
        if (this._lifetimeRemaining <= 0) {
            return 0;
        }
        if (this._lifetimeRealMode) {
            return this._lifetimeRemaining;
        }
        return this._lifetimeRemaining / this.vis.speed;
    }

    /**
     * @returns {boolean} whether the lifetime is over
     */
    isOver() {
        return this.realRemaining() <= 0;
    }

    /**
     * @returns {number} progress of the lifetime from 0.0 (start) to 1.0 (over)
     */
    progress() {
        if (this._lifetimeRemaining <= 0) {
            return 1.0;
        }
        return 1.0 - this._lifetimeRemaining / this._lifetime;
    }
}
