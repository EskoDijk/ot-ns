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
import ActionBar from "./ActionBar";
import {Text} from "./wrapper";
import {
    FRAME_CONTROL_MASK_FRAME_TYPE,
    FRAME_TYPE_ACK,
    LOG_WINDOW_FONT_COLOR,
    MAX_SPEED,
    PAUSE_SPEED,
    STATUS_MSG_FONT_FAMILY,
    STATUS_MSG_FONT_SIZE,
    NODE_ID_INVALID,
    NODE_LABEL_FONT_FAMILY
} from "./consts";
import NodeState from "./NodeState"
import PixiFieldRenderer from "./pixi/PixiFieldRenderer";
import ThreeFieldRenderer from "./three/ThreeFieldRenderer";
import {Skin, SkinName, SetSkin, DEFAULT_SKIN_NAME} from "./skins";
import LogWindow, {LOG_WINDOW_WIDTH} from "./LogWindow";
import * as fmt from "./format_text"
import NodeWindow from "./NodeWindow";

const {
    OtDeviceRole, CommandRequest
} = require('../proto/visualize_grpc_pb.js');

var vis = null;
let ticker = PIXI.Ticker.shared;

export function Visualizer() {
    return vis;
}

/**
 * The visualizer: keeps the state of the simulation as reported by the gRPC event stream (the
 * NodeState objects in `nodes`, time, speed, options), sends commands back to the simulator,
 * owns the selection, the HUD (status line, action bar, log window, node window) and keyboard
 * handling. Drawing of the field (nodes, links, messages) is delegated to `field`, a
 * FieldRenderer (see FieldRenderer.js).
 */
export default class PixiVisualizer extends VObject {
    constructor(app, grpcServiceClient) {
        super();
        vis = this;

        this.app = app;
        this.grpcServiceClient = grpcServiceClient;
        this.speed = 1;
        this.curTime = 0;
        this.curSpeed = 1;
        this.nodes = {}; // node ID -> NodeState
        this.newNodePos = null;
        // visualization options, see the 'cv' CLI command; defaults must match types.DefaultVisualizationOptions()
        this.visOptions = {
            broadcastMessage: true, unicastMessage: true, ackMessage: false,
            routerTable: true, childTable: true, partitionId: true, skin: DEFAULT_SKIN_NAME,
            skinPreset: "", skinPresets: [],
        };
        // if set, this skin is used instead of the one selected by the simulator (for development)
        this.skinOverride = null;

        this.root = new PIXI.Container();
        // this.root.width =
        this.root.position.set(0, 20);
        this.root.eventMode = 'static';
        this.root.hitArea = new PIXI.Rectangle(0, 0, 3000, 3000);
        app.stage.addChild(this.root);

        this.setOnTap((e) => {
            this.onTapedStage(e)
        });

        this.nodeLogColor = {};

        // the field renderer draws the nodes, links and messages; its back layer is behind the
        // log window, its front layer in front of it.
        this.field = new PixiFieldRenderer(this);
        this.addChild(this.field.backLayer);

        this._logWindowStage = new PIXI.Container();
        this.addChild(this._logWindowStage);

        this.addChild(this.field.frontLayer);

        this.statusMsg = new PIXI.Text({text: "", style: {
            fontFamily: STATUS_MSG_FONT_FAMILY,
            fontSize: STATUS_MSG_FONT_SIZE,
            fontWeight: "bold"
        }});
        this.statusMsg.position.set(0, -this.statusMsg.height);
        this.addChild(this.statusMsg);

        this.actionBar = new ActionBar();
        this.addChild(this.actionBar);
        this.actionBar.position.set(10, 1000);
        this.actionBar.setDraggable();
        this.updateStatusMsg();

        this.nodeWindow = new NodeWindow();
        this.addChild(this.nodeWindow);
        this._selectedNodeId = 0;
        this._selectAddedNode = false;
        // current size of the drawing field, kept up to date by onResize()
        this._fieldWidth = window.innerWidth;
        this._fieldHeight = window.innerHeight;

        this.otVersion = "";
        this.otCommit = "";
        this.otCommitIdMsg = new Text("", {
            fill: "#0052ff",
            fontFamily: NODE_LABEL_FONT_FAMILY,
            fontSize: 12
        });
        this.otCommitIdMsg.position.set(this.statusMsg.x, this.statusMsg.y + this.statusMsg.height + 3);
        this.otCommitIdMsg.interactive = true;
        this.otCommitIdMsg.setOnTap((e) => {
            if (this.otCommit.length > 0) {
                window.open('https://github.com/openthread/openthread/tree/' + this.otCommit, '_blank');
            }
            e.stopPropagation();
        });
        this.addChild(this.otCommitIdMsg);

        this.titleText = new PIXI.Text({text: "", style: {
            fill: "#e69900",
            fontFamily: "Verdana",
            fontSize: 20,
            fontWeight: "bolder"
        }});
        this.titleText.position.set(0, 20);
        this.addChild(this.titleText);

        this.real = false;
        this._applyReal();
        this._resetIdleCheckTimer()
    }

    update(dt) {
        super.update(dt);
        this.field.drawLinks(this._computeLinks());
        this.field.update(dt);
    }

    showLogWindow() {
        if (!this.logWindow) {
            this.logWindow = new LogWindow();
            this._logWindowStage.addChild(this.logWindow._root);
            this._resetLogWindowPosition(this._fieldWidth, this._fieldHeight);

            this.log("Log window opened.")
        }
    }

    hideLogWindow() {
        if (this.logWindow) {
            this._logWindowStage.removeChild(this.logWindow._root);
            this.logWindow = null
        }
    }

    clearLogWindow() {
        if (this.logWindow) {
            this.logWindow.clear()
        }
    }

    _resetLogWindowPosition(width, height) {
        if (this.logWindow) {
            this.logWindow.position.set(width - LOG_WINDOW_WIDTH, 10);
            this.logWindow.resetLayout(width, height)
        }
    }

    _resetIdleCheckTimer() {
        if (this._idleCheckTimer) {
            this.cancelCallback(this._idleCheckTimer);
            delete this._idleCheckTimer
        }

        this._idleCheckTimer = this.addCallback(10, () => {
            console.error("idle timer fired, reloading ...");
            location.reload()
        })
    }

    visAdvanceTime(ts, speed) {
        this.curTime = ts;
        this.curSpeed = speed;
        this._resetIdleCheckTimer();
        this.updateStatusMsg()
    }

    visHeartbeat() {
        this._resetIdleCheckTimer()
    }

    stopIdleCheckTimer() {
        if (this._idleCheckTimer) {
            this.cancelCallback(this._idleCheckTimer);
            delete this._idleCheckTimer
        }
    }

    setOTVersion(version, commit) {
        this.otVersion = version;
        this.otCommit = commit;
        if (version != this.otVersion) {
            console.log("Set default OpenThread Version display: " + version);
        }
        if (commit != this.otCommit) {
            console.log("Set default OpenThread Commit display : " + commit);
        }
        this.displayOTVersion(version, commit)
    }

    displayOTVersion(version, commit) {
        let txt = "OpenThread Version: " + version;
        if (version.length == 0) {
            txt += "-"
        }
        if (commit.length>0) {
            txt += " (" + commit + ")"
        }
        this.otCommitIdMsg.text =  txt;
    }

    setReal(real) {
        if (this.real === real) {
            return;
        }
        this.real = real;
        this._applyReal();
        this.log(`Real-time simulation: ${real ? "ON" : "OFF"}`);
    }

    _applyReal() {
        this.actionBar.setAbilities({
            "speed": !this.real,
            "add": true,
            "del": true,
            "radio": true,
            "otbr": this.real,
        })
    }

    updateStatusMsg() {
        this.statusMsg.text = "OTNS2-Web | FPS=" + fmt.spacePad(Math.round(ticker.FPS), 3) + " | "
            + fmt.spacePad(this._getNodeCountByRole(OtDeviceRole.OT_DEVICE_ROLE_LEADER),3) + " leaders "
            + fmt.spacePad(this._getPartitionCount(),3) + " partitions "
            + fmt.spacePad(this._getNodeCountByRole(OtDeviceRole.OT_DEVICE_ROLE_ROUTER),3) + " routers "
            + fmt.spacePad(this._getNodeCountByRole(OtDeviceRole.OT_DEVICE_ROLE_CHILD),3) + " EDs "
            + fmt.spacePad(this._getNodeCountByRole(OtDeviceRole.OT_DEVICE_ROLE_DETACHED),3) + " detached"
            + " | SPEED=" + this.formatSpeed()
            + " | TIME=" + this.formatTime();
    }

    _getNodeCountByRole(role) {
        let count = 0;
        for (let nodeid in this.nodes) {
            let node = this.nodes[nodeid];
            if (node.role === role) {
                count += 1
            }
        }
        return count
    }

    _getPartitionCount() {
        let aPts = {};
        for (let nodeid in this.nodes) {
            let pts = this.nodes[nodeid].partition;
            if (pts > 0) {
                aPts[pts] = 1;
            }
        }
        return Object.keys(aPts).length;
    }

    log(text, color = LOG_WINDOW_FONT_COLOR) {
        console.log(text);
        if (this.logWindow) {
            this.logWindow.addLog(text, color)
        }
    }

    visAddNode(nodeId, x, y, z, radioRange, nodeType) {
        let node = new NodeState(nodeId, x, y, z, radioRange, nodeType);
        this.nodes[nodeId] = node;
        this.field.addNode(node);
        if (this._selectAddedNode) {
            this.setSelectedNode(nodeId);
            this._selectAddedNode = false;
        }

        let msg = `Added at (${x},${y},${z})`;
        msg += `, radio range ${radioRange}`
        this.logNode(nodeId, msg)
        this.onNodeUpdate(nodeId);
    }

    visSetNodeRloc16(nodeId, rloc16) {
        let node = this.nodes[nodeId];
        let oldRloc16 = node.rloc16;
        node.setRloc16(rloc16);
        if (oldRloc16 != rloc16) {
            this.field.updateNode(node);
            this.logNode(nodeId, `RLOC16 changed from ${fmt.formatRloc16(oldRloc16)} to ${fmt.formatRloc16(rloc16)}`)
            this.onNodeUpdate(nodeId);
        }
    }

    visSetNodeRole(nodeId, role) {
        let node = this.nodes[nodeId];
        let oldRole = node.role;
        node.setRole(role);
        if (oldRole != role) {
            this.field.updateNode(node);
            this.logNode(nodeId, `Role changed from ${fmt.roleToString(oldRole)} to ${fmt.roleToString(role)}`)
            this.onNodeUpdate(nodeId);
        }
    }

    visSetNodeMode(nodeId, mode) {
        let node = this.nodes[nodeId];
        let oldMode = node.nodeMode;
        node.setMode(mode);
        if (oldMode != mode) {
            this.field.updateNode(node);
            this.logNode(nodeId, `Mode changed from ${fmt.modeToString(oldMode)} to ${fmt.modeToString(mode)}`);
            this.onNodeUpdate(nodeId);
        }
    }

    visSetNetworkInfo(version, commit, real, nodeId, threadVersion) {
        if (nodeId<=0) { // if no nodeId given, it sets default network display.
            this.setOTVersion(version, commit);
            this.setReal(real);
        }else{
            let node = this.nodes[nodeId];
            if (node) {
                node.setOTVersion(version, commit);
                node.setThreadVersion(threadVersion);
                this.displayOTVersion(version, commit);
                this.onNodeUpdate(nodeId);
            }else{
                this.log(`visSetNetworkInfo(): node ${nodeId} not found`);
            }
        }
    }

    visDeleteNode(nodeId) {
        let node = this.nodes[nodeId];
        delete this.nodes[nodeId];
        this.field.removeNode(node);
        if (nodeId === this._selectedNodeId) {
            this.setSelectedNode(0);
        }
        this.logNode(nodeId, "Deleted")
        this.onNodeUpdate(nodeId);
    }

    visSetSpeed(speed) {
        this.speed = speed;
        this.actionBar.setSpeed(speed);
        this.log(`Speed set to ${speed}`);
        this.updateStatusMsg();
    }

    isPaused() {
        return this.speed <= PAUSE_SPEED
    }

    isMaxSpeed() {
        return this.speed >= MAX_SPEED
    }

    visSetNodePos(nodeId, x, y, z) {
        let node = this.nodes[nodeId];
        node.setPosition(x, y, z);
        this.field.moveNode(node);
        this.logNode(nodeId, `Moved to (${x},${y},${z})`)
        this.onNodeUpdate(nodeId);
    }

    visOnExtAddrChange(nodeId, extAddr) {
        this.nodes[nodeId].extAddr = extAddr;
        this.logNode(nodeId, `Extended Address set to ${fmt.formatExtAddr(extAddr)}`)
        this.onNodeUpdate(nodeId);
    }

    visOnNodeFail(nodeId) {
        this.nodes[nodeId].failed = true;
        this.field.updateNode(this.nodes[nodeId]);
        this.logNode(nodeId, "Radio is OFF")
        this.onNodeUpdate(nodeId);
        this._refreshActionBarIfSelected(nodeId);
    }

    visOnNodeRecover(nodeId) {
        this.nodes[nodeId].failed = false;
        this.field.updateNode(this.nodes[nodeId]);
        this.logNode(nodeId, "Radio is ON")
        this.onNodeUpdate(nodeId);
        this._refreshActionBarIfSelected(nodeId);
    }

    // the action bar shows state-dependent labels (e.g. the radio toggle) for the selected node.
    _refreshActionBarIfSelected(nodeId) {
        if (nodeId == this._selectedNodeId) {
            this.actionBar.refresh();
        }
    }

    /**
     * @returns {NodeState|null} the selected node, if any
     */
    getSelectedNode() {
        return this.nodes[this._selectedNodeId] || null;
    }

    visSetParent(nodeId, extAddr) {
        let parent = this.findNodeByExtAddr(extAddr);
        this.nodes[nodeId].parent = extAddr;
        if (parent) {
            this.nodes[nodeId].parentId = parent.id;
        }else {
            this.nodes[nodeId].parentId = NODE_ID_INVALID;
        }
        this.logNode(nodeId, `Parent set to ${this.formatExtAddrPretty(extAddr)}`)
        this.onNodeUpdate(nodeId);
    }

    visSetTitle(title, x, y, fontSize) {
        let oldTitleText = this.titleText.text;
        this.titleText.text = title;
        this.titleText.x = x;
        this.titleText.y = y;
        this.titleText.style.fontSize = fontSize;

        if (oldTitleText !== title) {
            this.log(`Title set to "${title}", position (${x},${y}), font size ${fontSize}`);
        }
    }

    visSend(srcId, dstId, mvInfo) {
        if (document.visibilityState !== "visible") {
            return;
        }

        let src = this.nodes[srcId];
        if (src == null) return;

        let frameType = mvInfo.getFrameControl() & FRAME_CONTROL_MASK_FRAME_TYPE;
        if (frameType === FRAME_TYPE_ACK) {
            this.field.showAck(src, mvInfo);
        } else if (dstId == -1) {
            this.field.showBroadcast(src, mvInfo);
        } else {
            this.field.showUnicast(src, this.nodes[dstId] || null, mvInfo);
        }

        if (src.txPowerLast != mvInfo.getPowerDbm() || src.channelLast != mvInfo.getChannel()) {
            src.txPowerLast = mvInfo.getPowerDbm();
            src.channelLast = mvInfo.getChannel();
            this.onNodeUpdate(srcId);
        }
    }

    visSetNodePartitionId(nodeId, partitionId) {
        let node = this.nodes[nodeId];
        let oldPartitionId = node.partition;
        node.partition = partitionId;
        if (oldPartitionId != partitionId) {
            this.field.updateNode(node);
            this.logNode(nodeId, `Partition changed from ${fmt.formatPartitionId(oldPartitionId)} to ${fmt.formatPartitionId(partitionId)}`)
            this.onNodeUpdate(nodeId);
        }
    }

    visShowDemoLegend(x, y, title) {
        console.error("ShowDemoLegend not implemented")
    }

    visCountDown(durationMs, title) {
        console.error("CountDown not implemented")
    }

    // user controls (buttons) to add a node
    ctrlAddNode(type) {
        this._selectAddedNode = true; // make the new node the selected one, once it gets added.
        if (this.newNodePos == null) {
            this.runCommand("add " + type);
        }else{
            this.runCommand("add " + type + " x " + this.newNodePos.x + " y " + this.newNodePos.y);
            this.newNodePos = null
        }
    }

    /**
     * Move a node in the simulator.
     * @param z {number|null} the height; null keeps the node's current height (2D renderer)
     */
    ctrlMoveNodeTo(nodeId, x, y, z, cb) {
        let cmd = "move " + nodeId + " " + Math.floor(x) + " " + Math.floor(y);
        if (z !== null) {
            cmd += " " + Math.floor(z);
        }
        this.runCommand(cmd, cb);
    }

    ctrlDeleteNode(nodeId) {
        this.runCommand("del " + nodeId);
    }

    ctrlSetNodeFailed(nodeId, failed) {
        this.runCommand("radio " + nodeId + " " + (failed ? "off" : "on"))
    }

    ctrlSetSpeed(speed) {
        this.runCommand("speed " + speed)
    }

    ctrlSetSkin(name) {
        this.runCommand("cv skin " + name)
    }

    runCommand(cmd, callback) {
        let req = new CommandRequest();
        req.setCommand(cmd);
        this.log(`> ${cmd}`);

        this.grpcServiceClient.command(req, {}, (err, resp) => {
                if (err !== null) {
                    this.log("Error: " + err.toLocaleString());
                    console.error("Error: " + err.toLocaleString());
                    if (callback) {
                        callback(err, [])
                    }
                }

                let output = resp.getOutputList();
                for (let i in output) {
                    console.log(output[i]);
                }

                if (callback) {
                    let errmsg = output.pop();

                    if (errmsg !== "Done") {
                        callback(new Error(errmsg), output)
                    } else {
                        callback(null, output)
                    }
                }
            }
        )
    }

    visSetVisualizationOptions(opts) {
        this.visOptions = opts;
        this.field.setPartitionVisible(opts.partitionId);
        this.setSkin(this.skinOverride || opts.skin);
        this.actionBar.refresh(); // the skin button shows the selected preset, which may change without a skin change
    }

    /**
     * Activate the named skin (see skins/index.js) and redraw the nodes and links, switching to
     * the skin's field renderer if it differs from the active one.
     * @returns {boolean} false if the name is unknown; the active skin is then kept. True otherwise.
     */
    setSkin(name) {
        if (name === SkinName()) {
            return true;
        }
        if (!SetSkin(name)) {
            console.error("unknown visualization skin '" + name + "', keeping skin '" + SkinName() + "'");
            return false;
        }
        if (Skin().renderer !== this.field.kind) {
            this._switchFieldRenderer(Skin().renderer);
        } else {
            this.field.applySkin();
        }
        return true;
    }

    /**
     * Replace the field renderer by one of the given kind ('pixi' or 'three'), re-adding all
     * nodes to it. Its layers take the place of the old ones in the drawing order.
     */
    _switchFieldRenderer(kind) {
        const old = this.field;
        const backIndex = this._root.getChildIndex(old.backLayer);
        const frontIndex = this._root.getChildIndex(old.frontLayer);
        this.removeChild(old.backLayer);
        this.removeChild(old.frontLayer);
        old.destroy();

        this.field = kind === 'three' ? new ThreeFieldRenderer(this) : new PixiFieldRenderer(this);
        this.addChildAt(this.field.backLayer, backIndex);
        this.addChildAt(this.field.frontLayer, frontIndex);
        for (let nodeid in this.nodes) {
            this.field.addNode(this.nodes[nodeid]);
        }
        this.field.setSelectedNode(this.getSelectedNode());
        this.field.onResize(this._fieldWidth, this._fieldHeight);
        this.log(`Field renderer: ${kind}`);
    }

    /**
     * Map a 32-bit partition ID to a 24-bit RGB color (Pixi rejects larger color values). Black is
     * reserved for 'no partition' (ID 0). Any other ID is hashed so that all 32 bits contribute to the
     * color and the result is never so dark that it looks black.
     */
    getPartitionColor(parid) {
        if (parid === 0) {
            return 0x000000;
        }
        let color = Math.imul(parid, 0x9E3779B1) >>> 8; // 24-bit multiplicative hash
        if ((color & 0xc0c0c0) === 0) { // all channels below 0x40: brighten
            color |= 0x404040;
        }
        return color;
    }

    setSelectedNode(id) {
        if (id === this._selectedNodeId) {
            return;
        }

        this._selectedNodeId = 0; // unselect

        let new_sel = this.nodes[id];
        if (new_sel) {
            this._selectedNodeId = id;
            this.displayOTVersion(new_sel.otVersion, new_sel.otCommit);
        }else{
            this.displayOTVersion(this.otVersion, this.otCommit); // back to default
        }

        this.field.setSelectedNode(new_sel || null);
        this.nodeWindow.showNode(new_sel);
        this.actionBar.setContext(new_sel || "any");
    }

    /**
     * Select the node that is `step` positions (+1 next, -1 previous) after the selected node in
     * the order of node IDs, wrapping around. With no node selected, +1 selects the lowest and
     * -1 the highest node ID.
     * @returns {boolean} true if a node was selected, false if there are no nodes.
     */
    selectAdjacentNode(step) {
        const ids = Object.keys(this.nodes).map(Number).sort((a, b) => a - b);
        if (ids.length === 0) {
            return false;
        }
        let idx = ids.indexOf(this._selectedNodeId);
        if (idx < 0) {
            idx = step > 0 ? 0 : ids.length - 1;
        } else {
            idx = (idx + step + ids.length) % ids.length;
        }
        this.setSelectedNode(ids[idx]);
        return true;
    }

    /**
     * Keyboard shortcuts: Tab / Shift+Tab select the next / previous node, Escape unselects,
     * Delete deletes the selected node, Space pauses/resumes the simulation. Key combinations
     * with Ctrl/Alt/Meta are left to the browser (e.g. Ctrl+R reload).
     */
    onKeyDown(e) {
        const t = e.target;
        if (t && (t.isContentEditable || t.tagName === 'INPUT' || (t.tagName === 'TEXTAREA' && !t.readOnly))) {
            return; // don't take keys away from an editable element
        }
        if (e.ctrlKey || e.altKey || e.metaKey) {
            return;
        }
        if (this.field.onKeyDown(e)) { // renderer-specific keys, e.g. camera views in 3D
            e.preventDefault();
            return;
        }
        switch (e.key) {
            case 'Escape':
                this.setSelectedNode(0);
                e.preventDefault();
                break;
            case 'Tab': {
                // Tab is only taken over while no page element has the focus, and only when there is a
                // node to select; otherwise the browser's own focus navigation (e.g. to the node window)
                // keeps working.
                const a = document.activeElement;
                if (a && a.tagName !== 'BODY' && a.tagName !== 'CANVAS') {
                    break;
                }
                if (this.selectAdjacentNode(e.shiftKey ? -1 : 1)) {
                    e.preventDefault();
                }
                break;
            }
            case 'Delete':
                if (this.actionBar.hasAbility("del")) {
                    this.deleteSelectedNode();
                    e.preventDefault();
                }
                break;
            case ' ':
                if (this.actionBar.hasAbility("speed")) { // not in -realtime mode
                    this.actionBar.actionTogglePauseResume();
                    e.preventDefault();
                }
                break;
            default:
                break;
        }
    }

    setSpeed(speed) {
        if (this.real) {
            console.error("setSpeed() not available in real-time mode");
            return
        }

        this.ctrlSetSpeed(speed)
    }

    deleteSelectedNode() {
        let sel = this.nodes[this._selectedNodeId];
        if (sel) {
            this.ctrlDeleteNode(sel.id)
        }
    }

    setSelectedNodeFailed(failed) {
        let sel = this.nodes[this._selectedNodeId];
        if (!sel) {
            return
        }

        this.ctrlSetNodeFailed(sel.id, failed)
    }

    clearAllNodes() {
        for (let id in this.nodes) {
            this.ctrlDeleteNode(id)
        }
    }

    // a tap that no node view handled: on empty space it unselects; a 3D renderer's nodes are
    // not Pixi objects, so ask it whether a node was tapped.
    onTapedStage(e) {
        if (this.field.nodeAt(e.global) === null) {
            this.setSelectedNode(0)
        }
    }

    /**
     * The links to draw, from the parent/child relations and router tables of the nodes.
     * @returns {Array<{kind: string, selected: boolean, from: NodeState, to: NodeState}>} see
     *          FieldRenderer.drawLinks(). `kind` is 'child' for a parent-child link, 'router' for
     *          a router-table link between two Routers, or 'neighbor' for another router-table
     *          link: FTD children (FED/REED) also report router-table entries for the Routers
     *          they hear.
     */
    _computeLinks() {
        const links = [];
        const addLink = (kind, from, to) => {
            const selected = from.id === this._selectedNodeId || to.id === this._selectedNodeId;
            links.push({kind: kind, selected: selected, from: from, to: to});
        };
        for (let nodeid in this.nodes) {
            let node = this.nodes[nodeid];
            if (node.parent) {
                let parent = this.findNodeByExtAddr(node.parent);
                if (parent !== null) {
                    addLink('child', node, parent);
                }
            }
            for (let extaddr in node.children) {
                let child = this.findNodeByExtAddr(extaddr);
                if (child) {
                    addLink('child', node, child);
                }
            }
            for (let extaddr in node.neighbors) {
                let neighbor = this.findNodeByExtAddr(extaddr);
                if (neighbor) {
                    addLink(node.isRouter() && neighbor.isRouter() ? 'router' : 'neighbor', node, neighbor);
                }
            }
        }
        return links;
    }

    /**
     * find a node by extended address
     * @param extaddr
     * @returns {NodeState|null}
     */
    findNodeByExtAddr(extaddr) {
        for (let nodeid in this.nodes) {
            let node = this.nodes[nodeid];
            if (node.extAddr == extaddr) {
                return node
            }
        }
        return null
    }

    visAddRouterTable(nodeId, extaddr) {
        this.nodes[nodeId].addRouterTable(extaddr);
        this.logNode(nodeId, `Router table added: ${this.formatExtAddrPretty(extaddr)}`)
        this.onNodeUpdate(nodeId);
    }

    visRemoveRouterTable(nodeId, extaddr) {
        this.nodes[nodeId].removeRouterTable(extaddr);
        this.logNode(nodeId, `Router table removed: ${this.formatExtAddrPretty(extaddr)}`)
        this.onNodeUpdate(nodeId);
    }

    visAddChildTable(nodeId, extaddr) {
        this.nodes[nodeId].addChildTable(extaddr);
        this.logNode(nodeId, `Child table added: ${this.formatExtAddrPretty(extaddr)}`)
        let child = this.findNodeByExtAddr(extaddr);
        if (child && this.nodes[child.id]) {
            let extAddrParent = this.nodes[nodeId].extAddr;
            this.visSetParent(child.id,extAddrParent); // call from here because 'parent' push event is not emitted by OT.
        }
        this.onNodeUpdate(nodeId);
    }

    visRemoveChildTable(nodeId, extaddr) {
        this.nodes[nodeId].removeChildTable(extaddr);
        this.logNode(nodeId, `Child table removed: ${this.formatExtAddrPretty(extaddr)}`)
        this.onNodeUpdate(nodeId);
    }

    logNode(nodeId, msg) {
        let color = this.nodeLogColor[nodeId];
        if (typeof color == "undefined") {
            color = this.randomColor();
            this.nodeLogColor[nodeId] = color;
        }

        this.log(`Node ${nodeId}: ${msg}`, color)
    }

    onNodeUpdate(nodeId) {
        if(this._selectedNodeId==nodeId && nodeId > 0){
            this.nodeWindow.showNode(this.nodes[nodeId])
        }
    }

    randomColor() {
        let hue = Math.floor(Math.random() * 360);
        let color = `hsl(${hue}deg, 92%, 23%)`;
        return color;
    }

    formatExtAddrPretty(extAddr) {
        let node = this.findNodeByExtAddr(extAddr);
        if (node) {
            return `Node ${node.id}(${fmt.formatExtAddr(extAddr)})`
        } else {
            return fmt.formatExtAddr(extAddr);
        }
    }

    formatTime() {
        let us = this.curTime % 1000;
        let ms = Math.floor((this.curTime % 1000000) / 1000);
        let secs = Math.floor(this.curTime / 1000000);
        let d = Math.floor(secs / 86400);
        secs = secs % 86400;
        let h = Math.floor(secs / 3600);
        secs = secs % 3600;
        let m = Math.floor(secs / 60);
        secs = secs % 60;

        let str = "";
        if (d > 0) {
            str += d + "d";
        }
        str += h + "h" +
            m.toString().padStart(2, "0") + "m" +
            secs.toString().padStart(2, "0") + "s" +
            "  " + ms.toString().padStart(3," ") + "ms" +
            " " + us.toString().padStart(3," ") + "us";
        return str;
    }

    formatSpeed() {
        let s = this.curSpeed;
        if (s >= 0.9995) {
            return this.curSpeed.toFixed(1).toString().padStart(7, " ") + "     ";
        }else if (s >= 0.0009995) {
            return "    " + this.curSpeed.toFixed(3).toString() + "   ";
        }else {
            return "    " + this.curSpeed.toFixed(6).toString();
        }
    }

    onResize(width, height) {
        console.log("window resized to " + width + "," + height);
        this._fieldWidth = width;
        this._fieldHeight = height;
        this.actionBar.position.set(10, height - this.actionBar.height - 20 - 10);
        this._resetLogWindowPosition(width, height);
        this.field.onResize(width, height);
    }

}
