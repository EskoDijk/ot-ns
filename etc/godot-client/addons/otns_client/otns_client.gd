# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# OtnsClient: connects to the OTNS gRPC event stream through the gRPC-web proxy (the port that
# OTNS's web UI uses, base port - 2, e.g. 8998 for -listen localhost:9000; use 127.0.0.1, the
# proxy listens on IPv4 while 'localhost' may resolve to ::1), keeps the state of
# all nodes and emits a signal per change. Add one to the scene, connect the signals (or use an
# OtnsField node for a default drawing), and call send_command() to run OTNS CLI commands.
#
# Positions: OTNS units with z up; to_godot() converts to Godot meters (x, z, y) using the
# scale and origin of the floor plan the simulation uses (etc/floorplans/README.md).
class_name OtnsClient
extends Node

const PB := preload("res://addons/otns_client/proto/visualize_grpc_pb.gd")
const OtnsGrpcWebCall := preload("res://addons/otns_client/otns_grpc_web.gd")
const SERVICE := "/visualize_grpc_pb.VisualizeGrpcService/"
const RECONNECT_DELAY := 2.0
const FRAME_TYPE_MASK := 0x7
const FRAME_TYPE_ACK := 2
const EXT_ADDR_INVALID := -1  # uint64 all ones does not fit a GDScript int; -1 is what it decodes to

## The state of one simulated node, as reported by the events.
class OtnsNode:
	var id: int
	var type: String                # 'router', 'fed', 'sed', 'br', ... (OTNS node type)
	var x: int
	var y: int
	var z: int
	var radio_range: int
	var role: int = 0               # PB.OtDeviceRole
	var rloc16: int = 0xfffe
	var ext_addr: int = EXT_ADDR_INVALID
	var partition_id: int = 0
	var failed: bool = false
	var parent: int = EXT_ADDR_INVALID   # ext addr of the parent
	var children: Dictionary = {}   # ext addr -> true
	var neighbors: Dictionary = {}  # ext addr -> true (router table)
	var full_thread_device: bool = true
	var rx_on_when_idle: bool = true

	func is_router() -> bool:
		return role == PB.OtDeviceRole.ROUTER or role == PB.OtDeviceRole.LEADER

	func role_name() -> String:
		match role:
			PB.OtDeviceRole.LEADER: return "leader"
			PB.OtDeviceRole.ROUTER: return "router"
			PB.OtDeviceRole.CHILD: return "child"
			PB.OtDeviceRole.DETACHED: return "detached"
			_: return "disabled"

@export var host: String = "127.0.0.1"   ## grpcwebproxy binds IPv4; 'localhost' may resolve to ::1
@export var port: int = 8998
@export var auto_connect: bool = true      ## connect when the node is ready
@export var auto_reconnect: bool = true    ## reconnect after a lost or refused connection
@export var max_events_per_frame: int = 400      ## decode budget per frame; the rest waits for the next frame
@export var max_sends_per_frame: int = 20        ## 'Send' (frame sent) events decoded per frame; more are dropped
@export var units_per_meter: float = 10.0        ## floor plan 'unitsPerMeter'
@export var origin: Vector2 = Vector2(0, 0)      ## floor plan 'origin' (OTNS units)

signal connected()
signal disconnected(reason: String)
signal node_added(node: OtnsNode)
signal node_removed(node: OtnsNode)
signal node_updated(node: OtnsNode)   ## role, mode, RLOC16, ext addr, partition, failed
signal node_moved(node: OtnsNode)
signal links_changed()                ## a router/child table or parent changed
signal message_sent(src: OtnsNode, dst: OtnsNode, kind: String, mv_info)  ## kind: broadcast, unicast, ack; dst null unless unicast to a known node
signal time_advanced(timestamp_us: int, speed: float)
signal network_info(version: String, commit: String, real: bool)
signal title_set(title: String)
signal command_done(command: String, output: Array[String], err: String)
signal event_received(event)          ## every decoded VisualizeEvent (PB.VisualizeEvent)

var nodes: Dictionary = {}            ## node ID -> OtnsNode
var sim_time_us: int = 0
var speed: float = 1.0
var is_connected: bool = false

var dropped_sends: int = 0            ## Send events dropped for lack of decode budget (see max_sends_per_frame)
var _stream: OtnsGrpcWebCall = null
var _reconnect_at: float = 0.0
var _commands: Array = []             # [{call, command}]
var _pending: Array[PackedByteArray] = []   # received payloads not yet decoded

# protobuf key bytes of the first field of a VisualizeEvent, to sort payloads before decoding:
# (field number << 3) | wire type, as a varint. Send = field 17, AdvanceTime = 12, Heartbeat = 19.
const KEY_SEND := 0x8A         # 17 << 3 | 2 = 138 = 0x8A 0x01
const KEY_ADVANCE_TIME := 0x62 # 12 << 3 | 2 = 98
const KEY_HEARTBEAT := 0x9A    # 19 << 3 | 2 = 154 = 0x9A 0x01


func _ready() -> void:
	if auto_connect:
		connect_to_otns()


func connect_to_otns() -> void:
	_close_stream("")
	_stream = OtnsGrpcWebCall.new()
	_stream.start(host, port, SERVICE + "Visualize", PB.VisualizeRequest.new().to_bytes())


func disconnect_from_otns() -> void:
	auto_reconnect = false
	_close_stream("closed")


## Run an OTNS CLI command (e.g. "move 3 100 200 30"); command_done is emitted with its output.
func send_command(command: String) -> void:
	var req := PB.CommandRequest.new()
	req.set_command(command)
	var call := OtnsGrpcWebCall.new()
	call.start(host, port, SERVICE + "Command", req.to_bytes())
	_commands.append({"call": call, "command": command})


## OTNS position (units, z up) -> Godot position (meters, y up).
func to_godot(x: float, y: float, z: float) -> Vector3:
	return Vector3((x - origin.x) / units_per_meter, z / units_per_meter, (y - origin.y) / units_per_meter)


func node_position(node: OtnsNode) -> Vector3:
	return to_godot(node.x, node.y, node.z)


## Godot position -> OTNS units, e.g. for a 'move' command.
func to_otns(p: Vector3) -> Vector3i:
	return Vector3i(roundi(p.x * units_per_meter + origin.x), roundi(p.z * units_per_meter + origin.y), roundi(p.y * units_per_meter))


func find_node_by_ext_addr(ext_addr: int) -> OtnsNode:
	if ext_addr == EXT_ADDR_INVALID:
		return null
	for node in nodes.values():
		if node.ext_addr == ext_addr:
			return node
	return null


## The links to draw: [{kind: 'child'|'router'|'neighbor', a: OtnsNode, b: OtnsNode}], as the
## web visualizer derives them from the parent, child table and router table of each node.
func compute_links() -> Array:
	var links: Array = []
	for node in nodes.values():
		var parent := find_node_by_ext_addr(node.parent)
		if parent != null:
			links.append({"kind": "child", "a": node, "b": parent})
		for ext_addr in node.children.keys():
			var child := find_node_by_ext_addr(ext_addr)
			if child != null:
				links.append({"kind": "child", "a": node, "b": child})
		for ext_addr in node.neighbors.keys():
			var neighbor := find_node_by_ext_addr(ext_addr)
			if neighbor != null:
				var kind := "router" if node.is_router() and neighbor.is_router() else "neighbor"
				links.append({"kind": kind, "a": node, "b": neighbor})
	return links


func _process(_delta: float) -> void:
	_poll_stream()
	_poll_commands()


func _poll_stream() -> void:
	if _stream == null:
		if auto_reconnect and _reconnect_at > 0.0 and Time.get_ticks_msec() / 1000.0 >= _reconnect_at:
			connect_to_otns()
		return
	_pending.append_array(_stream.poll())
	# Decoding is pure GDScript and the stream can carry thousands of 'Send' events per second
	# at high simulation speeds: keep the frame time bounded by a decode budget, decode only the
	# last AdvanceTime of a batch, skip heartbeats, and drop Send events beyond their budget.
	var decoded := 0
	var sends := 0
	var last_time: PackedByteArray = PackedByteArray()
	var keep: Array[PackedByteArray] = []
	for i in _pending.size():
		var payload := _pending[i]
		if decoded >= max_events_per_frame:
			keep.append(payload)
			continue
		if payload.size() >= 2 and payload[0] == KEY_SEND and payload[1] == 0x01:
			if sends >= max_sends_per_frame:
				dropped_sends += 1
				continue
			sends += 1
		elif payload.size() >= 1 and payload[0] == KEY_ADVANCE_TIME:
			last_time = payload
			continue
		elif payload.size() >= 2 and payload[0] == KEY_HEARTBEAT and payload[1] == 0x01:
			continue
		_decode(payload)
		decoded += 1
	_pending = keep
	if last_time.size() > 0:
		_decode(last_time)
	if _stream.state == OtnsGrpcWebCall.State.FAILED or _stream.state == OtnsGrpcWebCall.State.DONE:
		var reason := _stream.error if _stream.error != "" else "stream ended"
		_close_stream(reason)
		_reconnect_at = Time.get_ticks_msec() / 1000.0 + RECONNECT_DELAY


func _decode(payload: PackedByteArray) -> void:
	var event = PB.VisualizeEvent.new()
	if event.from_bytes(payload) != PB.PB_ERR.NO_ERRORS:
		push_warning("OTNS: undecodable event of %d bytes" % payload.size())
		return
	if not is_connected:
		is_connected = true
		connected.emit()
	_dispatch(event)
	event_received.emit(event)


func _close_stream(reason: String) -> void:
	_pending.clear()
	if _stream != null:
		_stream.close()
		_stream = null
	if is_connected or reason != "":
		is_connected = false
		for node in nodes.values():
			node_removed.emit(node)
		nodes.clear()
		links_changed.emit()
		if reason != "":
			disconnected.emit(reason)


func _poll_commands() -> void:
	var still: Array = []
	for pending in _commands:
		var call: OtnsGrpcWebCall = pending["call"]
		var payloads := call.poll()
		var output: Array[String] = []
		for payload in payloads:
			var resp = PB.CommandResponse.new()
			if resp.from_bytes(payload) == PB.PB_ERR.NO_ERRORS:
				output.append_array(resp.get_output())
		if call.state == OtnsGrpcWebCall.State.DONE or call.state == OtnsGrpcWebCall.State.FAILED:
			command_done.emit(pending["command"], output, call.error)
		else:
			still.append(pending)
	_commands = still


func _node(id: int) -> OtnsNode:
	return nodes.get(id)


func _dispatch(ev) -> void:
	var TC := PB.VisualizeEvent.TypeCase
	match ev.get_type_case():
		TC.ADD_NODE:
			var e = ev.get_add_node()
			var node := OtnsNode.new()
			node.id = e.get_node_id()
			node.type = e.get_node_type()
			node.x = e.get_x()
			node.y = e.get_y()
			node.z = e.get_z()
			node.radio_range = e.get_radio_range()
			nodes[node.id] = node
			node_added.emit(node)
		TC.DELETE_NODE:
			var node := _node(ev.get_delete_node().get_node_id())
			if node != null:
				nodes.erase(node.id)
				node_removed.emit(node)
				links_changed.emit()
		TC.SET_NODE_POS:
			var e = ev.get_set_node_pos()
			var node := _node(e.get_node_id())
			if node != null:
				node.x = e.get_x()
				node.y = e.get_y()
				node.z = e.get_z()
				node_moved.emit(node)
		TC.SET_NODE_ROLE:
			var e = ev.get_set_node_role()
			var node := _node(e.get_node_id())
			if node != null:
				node.role = e.get_role()
				if node.role == PB.OtDeviceRole.DISABLED or node.role == PB.OtDeviceRole.DETACHED:
					node.parent = EXT_ADDR_INVALID
				node_updated.emit(node)
				links_changed.emit()
		TC.SET_NODE_RLOC_16:
			var e = ev.get_set_node_rloc16()
			var node := _node(e.get_node_id())
			if node != null:
				node.rloc16 = e.get_rloc16()
				node_updated.emit(node)
		TC.ON_EXT_ADDR_CHANGE:
			var e = ev.get_on_ext_addr_change()
			var node := _node(e.get_node_id())
			if node != null:
				node.ext_addr = e.get_ext_addr()
				node_updated.emit(node)
				links_changed.emit()
		TC.SET_NODE_PARTITION_ID:
			var e = ev.get_set_node_partition_id()
			var node := _node(e.get_node_id())
			if node != null:
				node.partition_id = e.get_partition_id()
				node_updated.emit(node)
		TC.SET_NODE_MODE:
			var e = ev.get_set_node_mode()
			var node := _node(e.get_node_id())
			if node != null:
				var mode = e.get_node_mode()
				node.full_thread_device = mode.get_full_thread_device()
				node.rx_on_when_idle = mode.get_rx_on_when_idle()
				node_updated.emit(node)
		TC.ON_NODE_FAIL:
			var node := _node(ev.get_on_node_fail().get_node_id())
			if node != null:
				node.failed = true
				node_updated.emit(node)
		TC.ON_NODE_RECOVER:
			var node := _node(ev.get_on_node_recover().get_node_id())
			if node != null:
				node.failed = false
				node_updated.emit(node)
		TC.SET_PARENT:
			var e = ev.get_set_parent()
			var node := _node(e.get_node_id())
			if node != null:
				node.parent = e.get_ext_addr()
				links_changed.emit()
		TC.ADD_ROUTER_TABLE:
			var e = ev.get_add_router_table()
			var node := _node(e.get_node_id())
			if node != null:
				node.neighbors[e.get_ext_addr()] = true
				links_changed.emit()
		TC.REMOVE_ROUTER_TABLE:
			var e = ev.get_remove_router_table()
			var node := _node(e.get_node_id())
			if node != null:
				node.neighbors.erase(e.get_ext_addr())
				links_changed.emit()
		TC.ADD_CHILD_TABLE:
			var e = ev.get_add_child_table()
			var node := _node(e.get_node_id())
			if node != null:
				node.children[e.get_ext_addr()] = true
				var child := find_node_by_ext_addr(e.get_ext_addr())
				if child != null:
					child.parent = node.ext_addr  # OT emits no parent event; derive it as the web UI does
				links_changed.emit()
		TC.REMOVE_CHILD_TABLE:
			var e = ev.get_remove_child_table()
			var node := _node(e.get_node_id())
			if node != null:
				node.children.erase(e.get_ext_addr())
				links_changed.emit()
		TC.SEND:
			var e = ev.get_send()
			var src := _node(e.get_src_id())
			if src != null:
				var mv = e.get_mv_info()
				var kind := "unicast"
				var dst: OtnsNode = null
				if (mv.get_frame_control() & FRAME_TYPE_MASK) == FRAME_TYPE_ACK:
					kind = "ack"
				elif e.get_dst_id() == -1:
					kind = "broadcast"
				else:
					dst = _node(e.get_dst_id())
				message_sent.emit(src, dst, kind, mv)
		TC.ADVANCE_TIME:
			var e = ev.get_advance_time()
			sim_time_us = e.get_timestamp()
			speed = e.get_speed()
			time_advanced.emit(sim_time_us, speed)
		TC.SET_SPEED:
			speed = ev.get_set_speed().get_speed()
			time_advanced.emit(sim_time_us, speed)
		TC.SET_NETWORK_INFO:
			var e = ev.get_set_network_info()
			if e.get_node_id() <= 0:
				network_info.emit(e.get_version(), e.get_commit(), e.get_real())
		TC.SET_TITLE:
			title_set.emit(ev.get_set_title().get_title())
		_:
			pass  # heartbeat, count down, demo legend, visualization options: not needed here
