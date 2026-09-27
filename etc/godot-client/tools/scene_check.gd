# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Headless check of the luminaire mapping against the full Bistro main scene, without OTNS:
# loads MainScene.tscn, lets OtnsLuminaires resolve the mapping, and reports per kind how many
# luminaires have a light and a glowing (emissive) mesh. Run inside the Bistro project:
#   godot4 --headless --path <bistro project> -s otns/scene_check.gd
extends SceneTree

const OtnsLuminairesScript := preload("res://addons/otns_client/otns_luminaires.gd")

var frames := 0


func _initialize() -> void:
	var t0 := Time.get_ticks_msec()
	var scene: Node = load("res://MainScene.tscn").instantiate()
	root.add_child(scene)
	print("MainScene loaded in %.1f s" % ((Time.get_ticks_msec() - t0) / 1000.0))


func _process(_delta: float) -> bool:
	frames += 1
	if frames < 3:
		return false
	var scene := root.get_child(root.get_child_count() - 1)
	var lum = scene.get_node_or_null("OtnsLuminaires")
	if lum == null:
		print("no OtnsLuminaires node in MainScene")
		quit(1)
		return true
	var mapping = JSON.parse_string(FileAccess.get_file_as_string("res://otns/bistro-lights.json"))
	var kinds := {}
	for light in mapping["lights"]:
		var k: String = light["kind"]
		if not kinds.has(k):
			kinds[k] = {"total": 0, "resolved": 0, "with_light": 0, "with_mesh": 0}
		kinds[k]["total"] += 1
		var t: Dictionary = lum.targets.get(int(light["id"]), {})
		if t.is_empty():
			continue
		kinds[k]["resolved"] += 1
		if not t["lights"].is_empty():
			kinds[k]["with_light"] += 1
		if not t["meshes"].is_empty():
			kinds[k]["with_mesh"] += 1
	for k in kinds:
		print("%-12s %s" % [k, kinds[k]])
	# lights without a glowing mesh: how far is the nearest emissive mesh, and which one?
	for light in mapping["lights"]:
		var t: Dictionary = lum.targets.get(int(light["id"]), {})
		if t.is_empty() or t["lights"].is_empty() or not t["meshes"].is_empty():
			continue
		var at: Vector3 = t["lights"][0].global_position
		var best := INF
		var best_name := ""
		for e in lum._emissive_meshes:
			var d: float = e["center"].distance_to(at)
			if d < best:
				best = d
				best_name = "%s (%s)%s" % [e["mesh"].name, lum._base_material(e["mesh"]).resource_path.get_file(), " used" if lum._emissive_used.has(e["mesh"]) else ""]
		print("   no mesh: %s -> nearest %.1f m: %s" % [light["name"], best, best_name])
	quit(0)
	return true
