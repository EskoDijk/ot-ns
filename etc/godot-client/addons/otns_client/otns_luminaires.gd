# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# OtnsLuminaires: drives lights of an existing Godot scene from OTNS node state. A JSON mapping
# (e.g. etc/floorplans/bistro-lights.json, made by bistro_lights.py) lists for each OTNS node ID
# the scene node that is its luminaire:
#   - a Light3D, or a node with a Light3D somewhere below it (street lamp, lantern): the light's
#     'visible' is switched;
#   - a MeshInstance3D with an emissive material: its material's emission is switched (a
#     duplicate of the material is used, the shared one stays as it is);
#   - a MeshInstance3D path with a '#k' suffix: part k of the mesh, where parts are the connected
#     components of the mesh in the order bistro_lights.py numbers them (by mean x, then z of
#     the part's vertices); the mesh is split into one MeshInstance3D per part at load time.
# Rules: a failed node (radio off) always switches its light off; otherwise the light is on when
# the node is attached (Child, Router or Leader) and off when detached or disabled, unless
# off_when_detached is false. Luminaires whose node the simulation does not know keep the
# scene's own state.
class_name OtnsLuminaires
extends Node

const PB := preload("res://addons/otns_client/proto/visualize_grpc_pb.gd")
const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")
const OFF_ALBEDO_FACTOR := 0.35   # darken an emissive mesh's albedo when its light is off

@export var client_path: NodePath                    ## the OtnsClient node; empty: sibling 'OtnsClient'
@export_file("*.json") var mapping_file: String = "" ## the lights JSON (res:// or absolute path)
@export var scene_root: NodePath = ".."              ## mapping node paths are relative to this node
@export var off_when_detached: bool = true
@export var verbose: bool = false

signal luminaire_changed(node_id: int, on: bool)

var client: OtnsClientScript
var targets: Dictionary = {}   # node ID -> {"lights": [Light3D], "meshes": [MeshInstance3D], "materials": [StandardMaterial3D], "bases": [Material]}
var states: Dictionary = {}    # node ID -> bool (on)
var _split_cache: Dictionary = {}   # MeshInstance3D path -> Array[MeshInstance3D] parts
var _refresh_timer := 0.0


func _ready() -> void:
	if client_path.is_empty():
		client = get_parent().get_node_or_null("OtnsClient") as OtnsClientScript
	else:
		client = get_node_or_null(client_path) as OtnsClientScript
	if client == null:
		push_error("OtnsLuminaires: no OtnsClient node found (set client_path)")
		return
	_load_mapping()
	client.node_added.connect(_apply)
	client.node_updated.connect(_apply)
	client.node_removed.connect(_forget)
	for node in client.nodes.values():
		_apply(node)


## Number of luminaires resolved to scene nodes.
func resolved_count() -> int:
	return targets.size()


func is_on(node_id: int) -> bool:
	return states.get(node_id, false)


func _load_mapping() -> void:
	if mapping_file == "":
		push_error("OtnsLuminaires: mapping_file not set")
		return
	var f := FileAccess.open(mapping_file, FileAccess.READ)
	if f == null:
		push_error("OtnsLuminaires: cannot read " + mapping_file)
		return
	var data = JSON.parse_string(f.get_as_text())
	if typeof(data) != TYPE_DICTIONARY or not data.has("lights"):
		push_error("OtnsLuminaires: no 'lights' list in " + mapping_file)
		return
	var root := get_node_or_null(scene_root)
	if root == null:
		push_error("OtnsLuminaires: scene_root not found")
		return
	var missing := 0
	for light in data["lights"]:
		var target := _resolve(root, str(light["name"]))
		if target.is_empty():
			missing += 1
			if verbose:
				push_warning("OtnsLuminaires: %s not found" % light["name"])
			continue
		targets[int(light["id"])] = target
	print("OtnsLuminaires: %d of %d luminaires resolved (%d missing)" % [targets.size(), data["lights"].size(), missing])


## Find the lights and emissive meshes of a mapping entry below `root`.
func _resolve(root: Node, name: String) -> Dictionary:
	var part := -1
	var path := name
	var hash_at := name.rfind("#")
	if hash_at >= 0:
		part = int(name.substr(hash_at + 1))
		path = name.substr(0, hash_at)
	var node := root.get_node_or_null(path)
	if node == null:
		return {}
	var lights: Array = []
	var meshes: Array = []
	if part >= 0:
		if not (node is MeshInstance3D):
			return {}
		var parts := _split_parts(node as MeshInstance3D)
		if part >= parts.size():
			return {}
		meshes.append(parts[part])
	elif node is Light3D:
		lights.append(node)
	elif node is MeshInstance3D:
		meshes.append(node)
	else:
		_collect_lights(node, lights)
		if lights.is_empty():
			return {}
	# per-target material duplicates for the emissive meshes
	var materials: Array = []
	var bases: Array = []
	for mesh in meshes:
		var base := _base_material(mesh)
		if base is StandardMaterial3D:
			var dup: StandardMaterial3D = base.duplicate()
			mesh.set_surface_override_material(0, dup)
			materials.append(dup)
			bases.append(base)
		else:
			materials.append(null)
			bases.append(base)
	return {"lights": lights, "meshes": meshes, "materials": materials, "bases": bases}


func _collect_lights(node: Node, out: Array) -> void:
	for child in node.get_children():
		if child is Light3D:
			out.append(child)
		_collect_lights(child, out)


static func _base_material(mesh: MeshInstance3D) -> Material:
	var m := mesh.get_surface_override_material(0)
	if m == null and mesh.mesh != null and mesh.mesh.get_surface_count() > 0:
		m = mesh.mesh.surface_get_material(0)
	return m


## Split a mesh into its connected components: one child MeshInstance3D per part, in the order of
## the extractor (mean x, then mean z of the part's vertices); the original becomes invisible.
func _split_parts(mi: MeshInstance3D) -> Array:
	var key := str(mi.get_path())
	if _split_cache.has(key):
		return _split_cache[key]
	var parts: Array = []
	var mesh := mi.mesh
	if mesh == null or mesh.get_surface_count() == 0:
		return parts
	var mdt := MeshDataTool.new()
	if mdt.create_from_surface(mesh, 0) != OK:
		return parts
	var n := mdt.get_vertex_count()
	# merge vertices at equal positions, then union-find over the faces
	var key_index := {}
	var canon := PackedInt32Array()
	canon.resize(n)
	for i in n:
		var p := mdt.get_vertex(i)
		var k := Vector3i(roundi(p.x * 10000.0), roundi(p.y * 10000.0), roundi(p.z * 10000.0))
		if not key_index.has(k):
			key_index[k] = key_index.size()
		canon[i] = key_index[k]
	var parent := PackedInt32Array()
	parent.resize(key_index.size())
	for i in parent.size():
		parent[i] = i
	for f in mdt.get_face_count():
		var a := _find(parent, canon[mdt.get_face_vertex(f, 0)])
		for c in [1, 2]:
			var b := _find(parent, canon[mdt.get_face_vertex(f, c)])
			if a != b:
				parent[b] = a
	# faces per component, and the component's mean position
	var comp_faces := {}
	var comp_sum := {}
	var comp_count := {}
	for f in mdt.get_face_count():
		var r := _find(parent, canon[mdt.get_face_vertex(f, 0)])
		if not comp_faces.has(r):
			comp_faces[r] = []
			comp_sum[r] = Vector3.ZERO
			comp_count[r] = 0
		comp_faces[r].append(f)
	for i in n:
		var r := _find(parent, canon[i])
		if comp_sum.has(r):
			comp_sum[r] += mdt.get_vertex(i)
			comp_count[r] += 1
	var roots := comp_faces.keys()
	roots.sort_custom(func(a, b):
		var ma: Vector3 = comp_sum[a] / comp_count[a]
		var mb: Vector3 = comp_sum[b] / comp_count[b]
		return ma.x < mb.x if not is_equal_approx(ma.x, mb.x) else ma.z < mb.z)
	var material := _base_material(mi)
	for r in roots:
		var st := SurfaceTool.new()
		st.begin(Mesh.PRIMITIVE_TRIANGLES)
		for f in comp_faces[r]:
			for c in 3:
				var idx := mdt.get_face_vertex(f, c)
				st.set_normal(mdt.get_vertex_normal(idx))
				st.set_uv(mdt.get_vertex_uv(idx))
				st.add_vertex(mdt.get_vertex(idx))
		var part := MeshInstance3D.new()
		part.name = "part%d" % parts.size()
		part.mesh = st.commit()
		part.set_surface_override_material(0, material)
		part.layers = mi.layers
		mi.add_child(part)
		parts.append(part)
	mi.visible = true
	# the original surface stays as geometry of the parts only
	mi.mesh = null
	_split_cache[key] = parts
	if verbose:
		print("OtnsLuminaires: split %s into %d parts" % [mi.name, parts.size()])
	return parts


static func _find(parent: PackedInt32Array, a: int) -> int:
	while parent[a] != a:
		parent[a] = parent[parent[a]]
		a = parent[a]
	return a


func _wanted_state(node) -> bool:
	if node.failed:
		return false
	var attached: bool = node.role in [PB.OtDeviceRole.CHILD, PB.OtDeviceRole.ROUTER, PB.OtDeviceRole.LEADER]
	return attached or not off_when_detached


func _apply(node) -> void:
	var target: Dictionary = targets.get(node.id, {})
	if target.is_empty():
		return
	var on := _wanted_state(node)
	if states.get(node.id) == on:
		return
	states[node.id] = on
	_set_target(target, on)
	luminaire_changed.emit(node.id, on)
	if verbose:
		print("OtnsLuminaires: node %d -> %s" % [node.id, "on" if on else "off"])


func _forget(node) -> void:
	states.erase(node.id)


func _set_target(target: Dictionary, on: bool) -> void:
	for light in target["lights"]:
		light.visible = on
	for i in target["meshes"].size():
		var mat: StandardMaterial3D = target["materials"][i]
		var base = target["bases"][i]
		if mat == null:
			target["meshes"][i].visible = on
			continue
		# the base material may be switched by the scene itself (e.g. day/night): follow it
		var base_on: bool = base.emission_enabled if base is StandardMaterial3D else true
		mat.emission_enabled = on and base_on
		mat.albedo_color = base.albedo_color if on else base.albedo_color * OFF_ALBEDO_FACTOR


func _process(delta: float) -> void:
	# re-apply the emissive meshes now and then, so that a day/night switch of the scene's shared
	# materials is followed (the duplicates do not see it)
	_refresh_timer += delta
	if _refresh_timer < 1.0:
		return
	_refresh_timer = 0.0
	for id in states.keys():
		_set_target(targets[id], states[id])
