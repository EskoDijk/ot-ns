# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Headless check of the OTNS gRPC-web client: connects to a running OTNS, prints the first
# events and the node table, then exits. Exit code 0 on success, 1 on a connection failure or
# timeout.
#
#   godot4 --headless --path etc/godot-client -s tools/stream_test.gd -- --otns=127.0.0.1:8998 --events=20
extends SceneTree

const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")

var client: Node
var max_events := 20
var timeout_s := 30.0
var count := 0
var started := 0.0
var exit_code := 1
var _done := false


func _initialize() -> void:
	var host := "127.0.0.1"
	var port := 8998
	for arg in OS.get_cmdline_user_args():
		if arg.begins_with("--otns="):
			var hp := arg.substr(7).split(":")
			host = hp[0]
			if hp.size() > 1:
				port = int(hp[1])
		elif arg.begins_with("--events="):
			max_events = int(arg.substr(9))
		elif arg.begins_with("--timeout="):
			timeout_s = float(arg.substr(10))
	client = OtnsClientScript.new()
	client.host = host
	client.port = port
	client.auto_connect = false
	root.add_child(client)
	client.connected.connect(func(): print("connected to %s:%d" % [host, port]))
	client.disconnected.connect(func(reason): print("disconnected: " + reason))
	client.node_added.connect(func(n): print("add_node %d %s at (%d,%d,%d) rr %d -> godot %s" % [n.id, n.type, n.x, n.y, n.z, n.radio_range, client.node_position(n)]))
	client.node_updated.connect(func(n): print("node %d: role %s rloc16 %04X ext %X partition %X failed %s" % [n.id, n.role_name(), n.rloc16, n.ext_addr, n.partition_id, n.failed]))
	client.node_moved.connect(func(n): print("node %d moved to (%d,%d,%d)" % [n.id, n.x, n.y, n.z]))
	client.message_sent.connect(func(src, dst, kind, mv): print("send %s from %d to %s ch %d %d bytes" % [kind, src.id, str(dst.id) if dst != null else "-", mv.get_channel(), mv.get_frame_size_bytes()]))
	client.time_advanced.connect(func(ts, speed): print("time %d us speed %.1f" % [ts, speed]))
	client.event_received.connect(_on_event)
	client.connect_to_otns()
	started = Time.get_ticks_msec() / 1000.0
	print("waiting for %d events from %s:%d (timeout %.0f s)" % [max_events, host, port, timeout_s])


func _on_event(_event) -> void:
	count += 1
	if count >= max_events and not _done:
		_done = true
		print("--- %d events received; %d nodes; links: %d" % [count, client.nodes.size(), client.compute_links().size()])
		exit_code = 0
		quit(0)


func _process(_delta: float) -> bool:
	if Time.get_ticks_msec() / 1000.0 - started > timeout_s:
		print("--- timeout after %d events" % count)
		quit(1)
		return true
	return false
