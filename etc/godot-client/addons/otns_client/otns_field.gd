# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# OtnsField: a default 3D drawing of an OtnsClient's state: a sphere per node colored by role
# (the thread skin's palette of the web UI), a label with ID and RLOC16, links as thin
# translucent lines, and messages as small sparks moving from the sender. Use it as is, or as
# the example for a scene-specific drawing (e.g. luminaires of a building).
class_name OtnsField
extends Node3D

const PB := preload("res://addons/otns_client/proto/visualize_grpc_pb.gd")
const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")
const COLOR_ROUTER := Color(0.99, 0.31, 0.15)     # Thread orange
const COLOR_LEADER := Color(0.52, 0.60, 0.69)     # Thread grey
const COLOR_REED := Color(1.0, 0.63, 0.48)
const COLOR_END_DEVICE := Color(0.69, 0.75, 0.77)
const COLOR_DETACHED := Color(0.33, 0.43, 0.48)
const COLOR_FAILED := Color(0.46, 0.46, 0.46)
const COLOR_LINK_ROUTER := Color(0.99, 0.31, 0.15, 0.5)
const COLOR_LINK_CHILD := Color(0.33, 0.33, 0.33, 0.4)
const COLOR_MESSAGE := Color(0.08, 0.40, 0.75)
const COLOR_ACK := Color(0.68, 0.90, 0.44)
const MESSAGE_SECONDS := 0.5
const ROUTER_CAPABLE_TYPES := ["router", "reed", "ftd", "br", "otbr", "matter"]

@export var client_path: NodePath          ## the OtnsClient node; empty: the sibling named 'OtnsClient'
@export var node_radius: float = 0.25      ## meters
@export var show_links: bool = true
@export var show_labels: bool = true
@export var show_messages: bool = true

var client: OtnsClientScript
var _views: Dictionary = {}                # node ID -> Node3D
var _links_mesh: ImmediateMesh
var _messages: Array = []                  # [{mesh, from, to, t}]
var _sphere: SphereMesh
var _spark: SphereMesh
var _link_material: StandardMaterial3D
var _links_dirty := false


func _ready() -> void:
	if client_path.is_empty():
		client = get_parent().get_node_or_null("OtnsClient") as OtnsClientScript
	else:
		client = get_node_or_null(client_path) as OtnsClientScript
	if client == null:
		push_error("OtnsField: no OtnsClient node found (set client_path)")
		return
	_sphere = SphereMesh.new()
	_sphere.radius = node_radius
	_sphere.height = node_radius * 2
	_spark = SphereMesh.new()
	_spark.radius = node_radius * 0.4
	_spark.height = node_radius * 0.8
	_link_material = StandardMaterial3D.new()
	_link_material.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	_link_material.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	_link_material.vertex_color_use_as_albedo = true
	_links_mesh = ImmediateMesh.new()
	var links_instance := MeshInstance3D.new()
	links_instance.mesh = _links_mesh
	links_instance.material_override = _link_material
	add_child(links_instance)

	client.node_added.connect(_on_node_added)
	client.node_removed.connect(_on_node_removed)
	client.node_updated.connect(_on_node_updated)
	client.node_moved.connect(_on_node_moved)
	client.links_changed.connect(func(): _links_dirty = true)
	client.message_sent.connect(_on_message)
	for node in client.nodes.values():
		_on_node_added(node)
	_links_dirty = true


func node_color(node: OtnsClientScript.OtnsNode) -> Color:
	if node.failed:
		return COLOR_FAILED
	match node.role:
		PB.OtDeviceRole.LEADER:
			return COLOR_LEADER
		PB.OtDeviceRole.ROUTER:
			return COLOR_ROUTER
		PB.OtDeviceRole.CHILD:
			var router_capable := node.type in ROUTER_CAPABLE_TYPES or (node.full_thread_device and node.type == "")
			return COLOR_REED if router_capable else COLOR_END_DEVICE
		PB.OtDeviceRole.DETACHED:
			return COLOR_DETACHED
		_:
			return COLOR_FAILED


func _on_node_added(node: OtnsClientScript.OtnsNode) -> void:
	var view := Node3D.new()
	view.name = "node_%d" % node.id
	var mesh := MeshInstance3D.new()
	mesh.name = "body"
	mesh.mesh = _sphere
	var material := StandardMaterial3D.new()
	material.albedo_color = node_color(node)
	mesh.material_override = material
	view.add_child(mesh)
	if show_labels:
		var label := Label3D.new()
		label.name = "label"
		label.billboard = BaseMaterial3D.BILLBOARD_ENABLED
		label.no_depth_test = true
		label.pixel_size = 0.004
		label.position = Vector3(0, node_radius * 1.8, 0)
		label.modulate = Color(0.1, 0.1, 0.1)
		view.add_child(label)
	view.position = client.node_position(node)
	add_child(view)
	_views[node.id] = view
	_update_view(node)


func _on_node_removed(node: OtnsClientScript.OtnsNode) -> void:
	var view: Node3D = _views.get(node.id)
	if view != null:
		_views.erase(node.id)
		view.queue_free()
	_links_dirty = true


func _on_node_updated(node: OtnsClientScript.OtnsNode) -> void:
	_update_view(node)


func _on_node_moved(node: OtnsClientScript.OtnsNode) -> void:
	var view: Node3D = _views.get(node.id)
	if view != null:
		view.position = client.node_position(node)
	_links_dirty = true


func _update_view(node: OtnsClientScript.OtnsNode) -> void:
	var view: Node3D = _views.get(node.id)
	if view == null:
		return
	var body: MeshInstance3D = view.get_node("body")
	body.material_override.albedo_color = node_color(node)
	var scale := 1.0 if node.full_thread_device else (0.8 if node.rx_on_when_idle else 0.65)
	body.scale = Vector3.ONE * scale
	if show_labels:
		var label: Label3D = view.get_node("label")
		label.text = "%d|%04X" % [node.id, node.rloc16]


func _on_message(src: OtnsClientScript.OtnsNode, dst: OtnsClientScript.OtnsNode, kind: String, _mv) -> void:
	if not show_messages or kind == "broadcast":
		return  # a broadcast has no destination to fly to; a ring would go here
	var from := client.node_position(src)
	var to := from + Vector3(0, 0.5, 0) if kind == "ack" else (client.node_position(dst) if dst != null else from + Vector3(0, 0, 2))
	var mesh := MeshInstance3D.new()
	mesh.mesh = _spark
	var material := StandardMaterial3D.new()
	material.albedo_color = COLOR_ACK if kind == "ack" else COLOR_MESSAGE
	material.emission_enabled = true
	material.emission = material.albedo_color
	mesh.material_override = material
	mesh.position = from
	add_child(mesh)
	_messages.append({"mesh": mesh, "from": from, "to": to, "t": 0.0})


func _process(delta: float) -> void:
	if _links_dirty:
		_links_dirty = false
		_draw_links()
	var still: Array = []
	for m in _messages:
		m["t"] += delta / MESSAGE_SECONDS
		if m["t"] >= 1.0:
			m["mesh"].queue_free()
		else:
			m["mesh"].position = m["from"].lerp(m["to"], m["t"])
			still.append(m)
	_messages = still


func _draw_links() -> void:
	_links_mesh.clear_surfaces()
	if not show_links:
		return
	var links := client.compute_links()
	if links.is_empty():
		return
	_links_mesh.surface_begin(Mesh.PRIMITIVE_LINES)
	for link in links:
		var color: Color = COLOR_LINK_ROUTER if link["kind"] == "router" else COLOR_LINK_CHILD
		_links_mesh.surface_set_color(color)
		_links_mesh.surface_add_vertex(client.node_position(link["a"]))
		_links_mesh.surface_set_color(color)
		_links_mesh.surface_add_vertex(client.node_position(link["b"]))
	_links_mesh.surface_end()
