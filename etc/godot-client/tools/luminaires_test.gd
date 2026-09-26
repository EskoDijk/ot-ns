# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Headless check of OtnsLuminaires with the Bistro demo's light scenes, against a running OTNS
# that loaded etc/floorplans/bistro.yaml. Run inside the Bistro project (see install-addon.sh):
#   godot4 --headless --path <bistro project> -s otns/luminaires_test.gd -- --otns=127.0.0.1:8998
# Builds a scene tree with the demo's lantern and string-light scenes and stand-in street lamp
# lights at the mapping's paths, waits for the network, fails node 1's radio and checks its light.
extends SceneTree

const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")
const OtnsLuminairesScript := preload("res://addons/otns_client/otns_luminaires.gd")

var client
var lum
var phase := 0
var t := 0.0
var started := 0.0
var fails := 0
var _expected := 0
var _checked_resolved := false
var _last_report := -1
var _bulb_check_at := 0.0
var lamp_id := 1    # a street lamp node (from the mapping)
var bulb_id := 21   # a string bulb node (from the mapping)


func _check(ok: bool, what: String) -> void:
	print(("PASS " if ok else "FAIL ") + what)
	if not ok:
		fails += 1


func _initialize() -> void:
	var host := "127.0.0.1"
	var port := 8998
	for arg in OS.get_cmdline_user_args():
		if arg.begins_with("--otns="):
			var hp := arg.substr(7).split(":")
			host = hp[0]
			if hp.size() > 1:
				port = int(hp[1])
	var main := Node3D.new()
	main.name = "Main"
	root.add_child(main)
	# the mapping's paths: Night Lights/StreetLamps/<lamp> and Props/Section02/<scene>/...
	var night := Node3D.new()
	night.name = "Night Lights"
	main.add_child(night)
	var mapping = JSON.parse_string(FileAccess.get_file_as_string("res://otns/bistro-lights.json"))
	for light in mapping["lights"]:
		var name := str(light["name"])
		if name.begins_with("Night Lights/"):  # stand-in lights at the mapping's paths
			var group := name.get_base_dir().get_file()
			var group_node := night.get_node_or_null(group)
			if group_node == null:
				group_node = Node3D.new()
				group_node.name = group
				night.add_child(group_node)
			var l: Light3D = OmniLight3D.new() if light["kind"] != "spotlight" else SpotLight3D.new()
			l.name = name.get_file()
			group_node.add_child(l)
	var props := Node3D.new()
	props.name = "Props"
	main.add_child(props)
	var section := Node3D.new()
	section.name = "Section02"
	props.add_child(section)
	section.add_child(load("res://Scenes/Section02/Bistro_Lanterns.tscn").instantiate())
	section.add_child(load("res://Scenes/Section02/Bistro_StringLights.tscn").instantiate())

	client = OtnsClientScript.new()
	client.name = "OtnsClient"
	client.host = host
	client.port = port
	client.auto_connect = false
	main.add_child(client)
	lum = OtnsLuminairesScript.new()
	lum.name = "OtnsLuminaires"
	lum.mapping_file = "res://otns/bistro-lights.json"
	lum.scene_root = NodePath("..")
	lum.verbose = false
	main.add_child(lum)
	_expected = mapping["lights"].size()
	for light in mapping["lights"]:
		if light["kind"] == "streetlamp":
			lamp_id = int(light["id"])
			break
	for light in mapping["lights"]:
		if light["kind"] == "stringlight":
			bulb_id = int(light["id"])
			break
	client.command_done.connect(func(cmd, output, err): print("OTNS> %s -> %s %s" % [cmd, output, err]))
	client.connected.connect(func(): print("connected"))
	client.disconnected.connect(func(reason): print("disconnected: " + reason))
	client.connect_to_otns()
	client.send_command("speed")  # an early unary call, to see that commands work at all
	started = Time.get_ticks_msec() / 1000.0
	print("connecting to %s:%d" % [host, port])


func _process(delta: float) -> bool:
	t += delta
	var elapsed := Time.get_ticks_msec() / 1000.0 - started
	if _bulb_check_at > 0.0 and Time.get_ticks_msec() / 1000.0 > _bulb_check_at:
		_bulb_check_at = 0.0
		var bulb: MeshInstance3D = lum.targets[bulb_id]["meshes"][0]
		var mat: StandardMaterial3D = bulb.get_surface_override_material(0)
		var same: bool = mat == lum.targets[bulb_id]["materials"][0]
		var base: StandardMaterial3D = lum.targets[bulb_id]["bases"][0]
		_check(mat.emission_enabled, "string bulb of node %d" % bulb_id + " follows the base material's emission (night): mat.emission %s base.emission %s same-object %s refresh %.1f state %s" % [mat.emission_enabled, base.emission_enabled, same, lum._refresh_timer, lum.states.get(bulb_id)])
	if not _checked_resolved and t > 0.5:
		_checked_resolved = true
		_check(lum.resolved_count() == _expected, "all %d luminaires resolved (%d)" % [_expected, lum.resolved_count()])
	if int(elapsed) % 15 == 0 and int(elapsed) != _last_report:
		_last_report = int(elapsed)
		var pending := []
		for c in client._commands:
			pending.append("%s: state %d http %d err '%s'" % [c["command"], c["call"].state, c["call"].http_status, c["call"].error])
		print("... %d s: %d nodes, connected %s, pending commands %s" % [int(elapsed), client.nodes.size(), client.is_connected, pending])
	match phase:
		0:  # wait until the network formed
			var attached := 0
			for n in client.nodes.values():
				if n.role >= 2:
					attached += 1
			if client.nodes.size() >= _expected and attached >= _expected * 0.95:
				var on := 0
				for id in client.nodes.keys():
					if lum.is_on(id):
						on += 1
				_check(on == attached, "%d attached nodes -> %d luminaires on" % [attached, on])
				var lamp1: Light3D = lum.targets[lamp_id]["lights"][0]
				_check(lamp1.visible, "street lamp of node %d is on" % lamp_id)
				# string bulbs: emission follows the scene's shared material (off by day in the demo);
				# switch the base material on, as the demo does at night, and see the bulb follow
				var bulb: MeshInstance3D = lum.targets[bulb_id]["meshes"][0]
				var base: StandardMaterial3D = lum.targets[bulb_id]["bases"][0]
				_check(lum.is_on(bulb_id) and bulb.get_surface_override_material(0).albedo_color == base.albedo_color, "string bulb of node %d" % bulb_id + " is on (not darkened)")
				base.emission_enabled = true
				_bulb_check_at = Time.get_ticks_msec() / 1000.0 + 1.5
				client.send_command("radio %d off" % lamp_id)
				phase = 1
				t = 0.0
			elif elapsed > 300.0:
				_check(false, "timeout: %d nodes, %d attached" % [client.nodes.size(), attached])
				_finish()
		1:  # radio off -> light off
			if client.nodes.has(lamp_id) and client.nodes[lamp_id].failed:
				_check(not lum.is_on(lamp_id) and not lum.targets[lamp_id]["lights"][0].visible, "node %d failed -> street lamp off" % lamp_id)
				client.send_command("radio %d on" % lamp_id)
				phase = 2
				t = 0.0
			elif t > 10.0:
				_check(false, "node %d did not fail within 10 s" % lamp_id)
				_finish()
		2:  # radio on -> light on again (once attached)
			if client.nodes.has(lamp_id) and not client.nodes[lamp_id].failed and client.nodes[lamp_id].role >= 2:
				_check(lum.is_on(lamp_id), "node %d recovered -> street lamp on" % lamp_id)
				phase = 3
			elif t > 60.0:
				_check(false, "node %d did not recover within 60 s (role %s)" % [lamp_id, client.nodes[lamp_id].role_name()])
				phase = 3
		3:  # done, once the pending bulb check ran
			_finish()
	return false


func _finish() -> void:
	if _bulb_check_at > 0.0:
		return  # wait for the pending bulb check first
	print("--- %d failures" % fails)
	quit(1 if fails > 0 else 0)
