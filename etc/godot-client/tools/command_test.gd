# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Headless check of unary gRPC-web commands: sends a few CLI commands in sequence while the
# event stream runs, prints each result, exits 0 when all returned.
#   godot4 --headless --path etc/godot-client -s tools/command_test.gd -- --otns=127.0.0.1:8998
extends SceneTree

const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")

var client
var commands := ["speed", "nodes", "time", "speed", "radio 1 off", "radio 1 on"]
var next := 0
var done := 0
var t := 0.0
var last := 0.0


func _initialize() -> void:
	var host := "127.0.0.1"
	var port := 8998
	for arg in OS.get_cmdline_user_args():
		if arg.begins_with("--otns="):
			var hp := arg.substr(7).split(":")
			host = hp[0]
			if hp.size() > 1:
				port = int(hp[1])
	client = OtnsClientScript.new()
	client.host = host
	client.port = port
	root.add_child(client)
	client.connected.connect(func(): print("connected"))
	client.disconnected.connect(func(reason): print("disconnected: " + reason))
	client.command_done.connect(func(cmd, output, err):
		done += 1
		print("%.1f s: '%s' -> %s %s" % [t, cmd, output, err]))


func _process(delta: float) -> bool:
	t += delta
	if next < commands.size() and t - last >= 2.0:
		print("%.1f s: sending '%s' (%d nodes known)" % [t, commands[next], client.nodes.size()])
		client.send_command(commands[next])
		next += 1
		last = t
	if done >= commands.size():
		print("--- all %d commands returned" % done)
		quit(0)
		return true
	if t > 40.0:
		print("--- timeout: %d of %d commands returned; pending: %s" % [done, commands.size(),
			client._commands.map(func(c): return "%s state %d http %d '%s'" % [c["command"], c["call"].state, c["call"].http_status, c["call"].error])])
		quit(1)
		return true
	return false
