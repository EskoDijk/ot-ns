# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# Probe: which gRPC events does an application-layer 'send' produce? Sends UDP and CoAP test
# messages between two nodes and prints every Send event seen in the next seconds.
#   godot4 --headless --path etc/godot-client -s tools/app_message_probe.gd -- --otns=127.0.0.1:8998
extends SceneTree

const OtnsClientScript := preload("res://addons/otns_client/otns_client.gd")

var client
var steps := ["send udp 1 2 ds 21", "send udp 1 2 ds 41", "send udp 1 2-3 ds 61", "send coap 1 2 ds 21", "send coap 1 2 con ds 41"]
var step := -1
var t := 0.0
var last := -100.0
var sends: Array = []


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
	client.max_sends_per_frame = 1000
	root.add_child(client)
	client.command_done.connect(func(cmd, output, err): print("%.1f s: '%s' -> %s %s" % [t, cmd, output, err]))
	client.message_sent.connect(func(src, dst, kind, mv):
		if step >= 0:
			sends.append("%.2f s: %s %d->%s ch %d %d bytes fc 0x%04X seq %d" % [t, kind, src.id, str(dst.id) if dst != null else "-", mv.get_channel(), mv.get_frame_size_bytes(), mv.get_frame_control(), mv.get_seq()]))


func _process(delta: float) -> bool:
	t += delta
	if client.nodes.size() < 3 or (client.nodes.has(1) and client.nodes[1].role < 2):
		return false
	if t - last >= 6.0:
		if step >= 0:
			print("--- events after '%s': %d Send events" % [steps[step], sends.size()])
			for s in sends:
				print("   " + s)
			sends.clear()
		step += 1
		if step >= steps.size():
			quit(0)
			return true
		print("%.1f s: sending '%s'" % [t, steps[step]])
		client.send_command(steps[step])
		last = t
	return false
