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
// Registry of the visualization skins, selectable with the CLI command 'cv skin <name>'.
// The names must match types.VisualizationSkins on the Go side.

import ThreadSkin from "./thread";
import ClassicSkin from "./classic";
import Office3DSkin from "./office3d";

const SKINS = {
    thread: ThreadSkin,
    classic: ClassicSkin,
    office3d: Office3DSkin,
};

export const DEFAULT_SKIN_NAME = 'classic';

let skinName = DEFAULT_SKIN_NAME;
let skin = new ClassicSkin();

/**
 * @returns {Skin} the active skin
 */
export function Skin() {
    return skin;
}

export function SkinName() {
    return skinName;
}

export function SkinNames() {
    return Object.keys(SKINS);
}

/**
 * Activate the skin with the given name. Callers must then rebuild the skin-dependent
 * objects (see PixiVisualizer.visSetVisualizationOptions).
 * @returns {boolean} false if the name is unknown; the active skin is left unchanged.
 */
export function SetSkin(name) {
    if (!(name in SKINS)) {
        return false;
    }
    if (name !== skinName) {
        skinName = name;
        skin = new SKINS[name]();
    }
    return true;
}
