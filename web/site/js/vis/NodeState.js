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
// NodeState is the renderer-independent state of a simulated node, as reported by the simulator
// through the gRPC events. It holds no drawing objects; the active FieldRenderer keeps its own
// per-node view (see FieldRenderer.js).

import {NodeMode, OtDeviceRole} from '../proto/visualize_grpc_pb'
import {NODE_ID_INVALID, POWER_DBM_INVALID, EXT_ADDR_INVALID} from "./consts";

export default class NodeState {
    constructor(nodeId, x, y, z, radioRange, nodeType) {
        this.id = nodeId;
        this.type = nodeType;       // OTNS node type: 'router', 'fed', 'sed', 'br', ... (see types/types.go)
        this.x = x;                 // position in OTNS units; z is the height
        this.y = y;
        this.z = z;
        this.radioRange = radioRange;
        this.threadVersion = 0;
        this.extAddr = EXT_ADDR_INVALID;
        this.nodeMode = new NodeMode([true, true, true, true]);
        this.rloc16 = 0xfffe;
        this.routerId = NODE_ID_INVALID;
        this.childId = NODE_ID_INVALID;
        this.parent = 0;            // extended address of the parent, 0 if none
        this.parentId = NODE_ID_INVALID;
        this.role = OtDeviceRole.OT_DEVICE_ROLE_DISABLED;
        this.partition = 0;
        this.failed = false;        // radio switched off by the simulator ('radio' command)
        this.txPowerLast = POWER_DBM_INVALID;
        this.channelLast = -1;
        this.otVersion = "";
        this.otCommit = "";
        this.children = {};         // child table: ext addr -> 1
        this.neighbors = {};        // router table: ext addr -> 1
    }

    /**
     * @returns {string} the action bar context of a node, see ActionBar.setContext().
     */
    getActionContext() {
        return "node"
    }

    /**
     * @returns {boolean} whether the node is a Router or the Leader.
     */
    isRouter() {
        return this.role === OtDeviceRole.OT_DEVICE_ROLE_ROUTER || this.role === OtDeviceRole.OT_DEVICE_ROLE_LEADER;
    }

    /**
     * @returns the node state that a skin uses to draw the node, see skins/Skin.js.
     */
    skinState() {
        return {type: this.type, role: this.role, nodeMode: this.nodeMode, failed: this.failed};
    }

    setPosition(x, y, z) {
        this.x = x;
        this.y = y;
        this.z = z;
    }

    setRloc16(rloc16) {
        this.rloc16 = rloc16;
        this.routerId = rloc16 >> 10;
        this.childId = rloc16 & 0x01ff;
    }

    setRole(role) {
        this.role = role;
        if (role === OtDeviceRole.OT_DEVICE_ROLE_DISABLED || role === OtDeviceRole.OT_DEVICE_ROLE_DETACHED) {
            this.parent = NODE_ID_INVALID;
            this.parentId = NODE_ID_INVALID;
            this.childId = NODE_ID_INVALID;
            this.routerId = NODE_ID_INVALID;
        }
    }

    setMode(mode) {
        this.nodeMode = mode;
    }

    setThreadVersion(version) {
        this.threadVersion = version;
    }

    setOTVersion(version, commit) {
        this.otVersion = version;
        this.otCommit = commit;
    }

    addRouterTable(extaddr) {
        this.neighbors[extaddr] = 1
    }

    removeRouterTable(extaddr) {
        delete this.neighbors[extaddr]
    }

    addChildTable(extaddr) {
        this.children[extaddr] = 1
    }

    removeChildTable(extaddr) {
        delete this.children[extaddr]
    }
}
