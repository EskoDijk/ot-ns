# Copyright (c) 2026, The OTNS Authors. All rights reserved.
# BSD-3-Clause license, see the LICENSE file of the OTNS repository.
#
# A free-flying camera: hold the right mouse button to look around, W/A/S/D move, Q/E down/up,
# Shift is faster, mouse wheel changes the speed.
extends Camera3D

@export var speed: float = 8.0   # m/s
@export var mouse_sensitivity: float = 0.0025

var _yaw := 0.0
var _pitch := 0.0


func _ready() -> void:
	sync_angles()


## Take the yaw and pitch from the current rotation (after look_at() etc.).
func sync_angles() -> void:
	_yaw = rotation.y
	_pitch = rotation.x


func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventMouseButton:
		if event.button_index == MOUSE_BUTTON_RIGHT:
			Input.mouse_mode = Input.MOUSE_MODE_CAPTURED if event.pressed else Input.MOUSE_MODE_VISIBLE
		elif event.pressed and event.button_index == MOUSE_BUTTON_WHEEL_UP:
			speed *= 1.25
		elif event.pressed and event.button_index == MOUSE_BUTTON_WHEEL_DOWN:
			speed /= 1.25
	elif event is InputEventMouseMotion and Input.mouse_mode == Input.MOUSE_MODE_CAPTURED:
		_yaw -= event.relative.x * mouse_sensitivity
		_pitch = clamp(_pitch - event.relative.y * mouse_sensitivity, -1.5, 1.5)
		rotation = Vector3(_pitch, _yaw, 0)


func _process(delta: float) -> void:
	var dir := Vector3.ZERO
	if Input.is_physical_key_pressed(KEY_W): dir -= transform.basis.z
	if Input.is_physical_key_pressed(KEY_S): dir += transform.basis.z
	if Input.is_physical_key_pressed(KEY_A): dir -= transform.basis.x
	if Input.is_physical_key_pressed(KEY_D): dir += transform.basis.x
	if Input.is_physical_key_pressed(KEY_Q): dir -= Vector3.UP
	if Input.is_physical_key_pressed(KEY_E): dir += Vector3.UP
	if dir != Vector3.ZERO:
		var factor := 3.0 if Input.is_physical_key_pressed(KEY_SHIFT) else 1.0
		position += dir.normalized() * speed * factor * delta
