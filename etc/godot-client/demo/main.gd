# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Demo scene: an OtnsClient, an OtnsField drawing, a fly camera and a status line. The OTNS
# address can be given on the command line after '--': --otns=host:port. Press F to frame all nodes.
extends Node3D

const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")

@onready var client: OtnsClientScript = $OtnsClient
@onready var status: Label = $HUD/Status
@onready var camera: Camera3D = $Camera3D

var _title := ""


func _ready() -> void:
	for arg in OS.get_cmdline_user_args():
		if arg.begins_with("--otns="):
			var hp := arg.substr(7).split(":")
			client.host = hp[0]
			if hp.size() > 1:
				client.port = int(hp[1])
	client.connected.connect(func(): print("OTNS: connected to %s:%d" % [client.host, client.port]))
	client.disconnected.connect(func(reason): print("OTNS: disconnected: " + reason))
	client.title_set.connect(func(t): _title = t)
	client.command_done.connect(func(cmd, output, err): print("OTNS> %s: %s %s" % [cmd, " / ".join(output), err]))
	client.connect_to_otns()


func _process(_delta: float) -> void:
	var roles := {}
	for node in client.nodes.values():
		roles[node.role_name()] = roles.get(node.role_name(), 0) + 1
	var secs := client.sim_time_us / 1000000
	status.text = "OTNS %s:%d  %s  |  %d nodes %s  |  speed %.1f  time %dh%02dm%02ds  |  %s" % [
		client.host, client.port, "connected" if client.is_connected else "connecting...",
		client.nodes.size(), roles, client.speed, secs / 3600, (secs / 60) % 60, secs % 60, _title]


func _unhandled_key_input(event: InputEvent) -> void:
	if event.pressed and event.keycode == KEY_F:
		frame_nodes()


## Move the camera so that all nodes are in view.
func frame_nodes() -> void:
	if client.nodes.is_empty():
		return
	var aabb := AABB(client.node_position(client.nodes.values()[0]), Vector3.ZERO)
	for node in client.nodes.values():
		aabb = aabb.expand(client.node_position(node))
	var center := aabb.get_center()
	var extent: float = max(aabb.size.x, aabb.size.z, 5.0)
	camera.position = center + Vector3(0, extent * 0.7, extent * 1.0)
	camera.look_at(center)
	camera.sync_angles()
