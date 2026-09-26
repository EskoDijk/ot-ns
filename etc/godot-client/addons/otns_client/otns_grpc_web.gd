# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# A gRPC-web call over Godot's HTTPClient, as spoken by the grpcwebproxy that OTNS starts for
# its web UI (HTTP/1.1, content type application/grpc-web+proto). Works for server-streaming
# calls (the OTNS Visualize event stream) and unary calls (Command). It knows nothing about
# protobuf: poll() returns the raw message payloads; the caller decodes them.
#
# Wire format: the request body and the response body are sequences of frames, each 1 byte of
# flags, 4 bytes big-endian length, then the bytes. Flag 0x00 is a message, flag 0x80 marks the
# trailers ("grpc-status: 0" etc.) that end the stream.
class_name OtnsGrpcWebCall
extends RefCounted

enum State { IDLE, CONNECTING, REQUESTING, STREAMING, DONE, FAILED }

const CONTENT_TYPE := "application/grpc-web+proto"
const READ_CHUNK_SIZE := 65536

var state: State = State.IDLE
var error: String = ""
var grpc_status: int = -1     # from the trailers, -1 until they arrive
var http_status: int = 0
var _http := HTTPClient.new()
var _host: String
var _port: int
var _path: String
var _request: PackedByteArray
var _buffer := PackedByteArray()
var _trailers := ""


static func frame(message: PackedByteArray) -> PackedByteArray:
	var out := PackedByteArray()
	out.resize(5)
	out[0] = 0
	out.encode_u32(1, 0)
	# big-endian length
	var n := message.size()
	out[1] = (n >> 24) & 0xff
	out[2] = (n >> 16) & 0xff
	out[3] = (n >> 8) & 0xff
	out[4] = n & 0xff
	out.append_array(message)
	return out


## Start the call: POST `path` (e.g. "/visualize_grpc_pb.VisualizeGrpcService/Visualize") with
## one request message. Then call poll() every frame.
func start(host: String, port: int, path: String, request_message: PackedByteArray) -> void:
	_host = host
	_port = port
	_path = path
	_request = frame(request_message)
	_buffer.clear()
	_trailers = ""
	grpc_status = -1
	error = ""
	_http = HTTPClient.new()
	_http.read_chunk_size = READ_CHUNK_SIZE
	var err := _http.connect_to_host(host, port)
	if err != OK:
		_fail("connect_to_host(%s:%d): %s" % [host, port, error_string(err)])
		return
	state = State.CONNECTING


func close() -> void:
	_http.close()
	if state != State.DONE and state != State.FAILED:
		state = State.DONE


## Advance the call; returns the message payloads received since the previous poll (may be
## empty). Check `state` afterwards: DONE when the server ended the stream, FAILED on error.
func poll() -> Array[PackedByteArray]:
	var messages: Array[PackedByteArray] = []
	if state == State.DONE or state == State.FAILED or state == State.IDLE:
		return messages
	_http.poll()
	var status := _http.get_status()
	match state:
		State.CONNECTING:
			match status:
				HTTPClient.STATUS_CONNECTED:
					var headers := PackedStringArray([
						"Content-Type: " + CONTENT_TYPE, "Accept: " + CONTENT_TYPE,
						"X-Grpc-Web: 1", "X-User-Agent: otns-godot-client",
					])
					var err := _http.request_raw(HTTPClient.METHOD_POST, _path, headers, _request)
					if err != OK:
						_fail("request: " + error_string(err))
					else:
						state = State.REQUESTING
				HTTPClient.STATUS_RESOLVING, HTTPClient.STATUS_CONNECTING:
					pass
				_:
					_fail("cannot connect to %s:%d (status %d)" % [_host, _port, status])
		State.REQUESTING:
			if status == HTTPClient.STATUS_BODY or status == HTTPClient.STATUS_CONNECTED:
				if _http.has_response():
					http_status = _http.get_response_code()
					if http_status != 200:
						_fail("HTTP %d" % http_status)
						return messages
					state = State.STREAMING
					messages = _read(status)
			elif status != HTTPClient.STATUS_REQUESTING:
				_fail("request failed (status %d)" % status)
		State.STREAMING:
			messages = _read(status)
	return messages


func _read(status: int) -> Array[PackedByteArray]:
	var messages: Array[PackedByteArray] = []
	while status == HTTPClient.STATUS_BODY:
		var chunk := _http.read_response_body_chunk()
		if chunk.size() == 0:
			break  # nothing more right now
		_buffer.append_array(chunk)
		_http.poll()
		status = _http.get_status()
	# split the buffer into frames
	while _buffer.size() >= 5:
		var flags := _buffer[0]
		var length := (_buffer[1] << 24) | (_buffer[2] << 16) | (_buffer[3] << 8) | _buffer[4]
		if _buffer.size() < 5 + length:
			break
		var payload := _buffer.slice(5, 5 + length)
		_buffer = _buffer.slice(5 + length)
		if flags & 0x80:
			_trailers = payload.get_string_from_utf8()
			_parse_trailers()
			state = State.DONE
			_http.close()
			return messages
		messages.append(payload)
	if status == HTTPClient.STATUS_DISCONNECTED or status == HTTPClient.STATUS_CONNECTED:
		# the server closed the connection; without trailers that is an abnormal end
		if state != State.DONE:
			if grpc_status < 0:
				_fail("stream closed by the server without trailers")
	elif status == HTTPClient.STATUS_CONNECTION_ERROR:
		_fail("connection error")
	return messages


func _parse_trailers() -> void:
	for line in _trailers.split("\n"):
		var kv := line.strip_edges().split(":", true, 1)
		if kv.size() == 2 and kv[0].strip_edges().to_lower() == "grpc-status":
			grpc_status = int(kv[1].strip_edges())
		elif kv.size() == 2 and kv[0].strip_edges().to_lower() == "grpc-message" and kv[1].strip_edges() != "":
			error = kv[1].strip_edges().uri_decode()


func _fail(message: String) -> void:
	error = message
	state = State.FAILED
	_http.close()
