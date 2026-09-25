#!/usr/bin/env python3
#
# Copyright (c) 2026, The OTNS Authors.
# All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
# 1. Redistributions of source code must retain the above copyright
#    notice, this list of conditions and the following disclaimer.
# 2. Redistributions in binary form must reproduce the above copyright
#    notice, this list of conditions and the following disclaimer in the
#    documentation and/or other materials provided with the distribution.
# 3. Neither the name of the copyright holder nor the
#    names of its contributors may be used to endorse or promote products
#    derived from this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.
#
"""
Generate a small glTF binary (.glb) building model from a floor plan JSON file (see README.md),
as a test model for the 'model' entry of a floor plan: the same slabs, walls and door openings
as the visualizer draws procedurally, but as a glTF mesh, in meters, y up, with one node per
floor named after the floor. Usage:

    ./make_glb.py office-small.json office-small.glb

The .glb is written without any dependencies beyond the Python standard library.
"""
import json
import math
import struct
import sys

SLAB_THICKNESS = 0.15
MATERIALS = {
    "slab": [0.80, 0.83, 0.86, 0.9],
    "exterior": [0.87, 0.80, 0.68, 0.6],
    "interior": [0.62, 0.70, 0.78, 0.45],
}


class Mesh:
    """Triangles of one material: flat lists of positions, normals and indices."""

    def __init__(self):
        self.positions = []
        self.normals = []
        self.indices = []

    def add_box(self, w, h, d, center, angle):
        """A w x h x d box centered at `center` (x, y, z), rotated by `angle` about y."""
        ca, sa = math.cos(angle), math.sin(angle)
        faces = [  # normal, and the 4 corners (in local box coordinates, counter-clockwise seen from outside)
            ((0, 0, 1), [(-1, -1, 1), (1, -1, 1), (1, 1, 1), (-1, 1, 1)]),
            ((0, 0, -1), [(1, -1, -1), (-1, -1, -1), (-1, 1, -1), (1, 1, -1)]),
            ((1, 0, 0), [(1, -1, 1), (1, -1, -1), (1, 1, -1), (1, 1, 1)]),
            ((-1, 0, 0), [(-1, -1, -1), (-1, -1, 1), (-1, 1, 1), (-1, 1, -1)]),
            ((0, 1, 0), [(-1, 1, 1), (1, 1, 1), (1, 1, -1), (-1, 1, -1)]),
            ((0, -1, 0), [(-1, -1, -1), (1, -1, -1), (1, -1, 1), (-1, -1, 1)]),
        ]
        for normal, corners in faces:
            base = len(self.positions) // 3
            nx, ny, nz = normal
            rn = (nx * ca + nz * sa, ny, -nx * sa + nz * ca)
            for cx, cy, cz in corners:
                lx, ly, lz = cx * w / 2, cy * h / 2, cz * d / 2
                x = lx * ca + lz * sa + center[0]
                y = ly + center[1]
                z = -lx * sa + lz * ca + center[2]
                self.positions += [x, y, z]
                self.normals += list(rn)
            self.indices += [base, base + 1, base + 2, base, base + 2, base + 3]


def add_wall(meshes, material, frm, to, thickness, height, elevation, openings, door_height):
    dx, dy = to[0] - frm[0], to[1] - frm[1]
    length = math.hypot(dx, dy)
    if length <= 0:
        return
    angle = -math.atan2(dy, dx)

    def along(t):
        return frm[0] + dx * t / length, frm[1] + dy * t / length

    def piece(t0, t1, y0, h):
        mx, my = along((t0 + t1) / 2)
        meshes[material].add_box(t1 - t0, h, thickness, (mx, y0 + h / 2, my), angle)

    gaps = sorted([max(o["at"] - o["width"] / 2, 0), min(o["at"] + o["width"] / 2, length)] for o in openings)
    start = 0
    for g0, g1 in gaps:
        if g0 > start:
            piece(start, g0, elevation, height)
        if g1 > g0 and height > door_height:
            piece(g0, g1, elevation + door_height, height - door_height)
        start = max(start, g1)
    if length > start:
        piece(start, length, elevation, height)


def floor_meshes(plan, floor):
    meshes = {name: Mesh() for name in MATERIALS}
    thickness = plan.get("wallThickness", 0.2)
    door_height = plan.get("doorHeight", 2.1)
    elevation = floor.get("elevation", 0)
    height = floor.get("height", 3.0)
    w, d = floor.get("outline", [0, 0])
    if w > 0 and d > 0:
        meshes["slab"].add_box(w, SLAB_THICKNESS, d, (w / 2, elevation - SLAB_THICKNESS / 2, d / 2), 0)
        for a, b in [((0, 0), (w, 0)), ((w, 0), (w, d)), ((w, d), (0, d)), ((0, d), (0, 0))]:
            add_wall(meshes, "exterior", a, b, thickness, height, elevation, [], door_height)
    for wall in floor.get("walls", []):
        add_wall(meshes, "interior", wall["from"], wall["to"], wall.get("thickness", thickness),
                 wall.get("height", height), elevation, wall.get("openings", []), door_height)
    return meshes


def write_glb(plan, path):
    binary = bytearray()
    buffer_views, accessors, materials, gltf_meshes, nodes = [], [], [], [], []
    material_index = {}
    for name, rgba in MATERIALS.items():
        material_index[name] = len(materials)
        materials.append({"name": name, "doubleSided": True, "alphaMode": "BLEND" if rgba[3] < 1 else "OPAQUE",
                          "pbrMetallicRoughness": {"baseColorFactor": rgba, "metallicFactor": 0.0, "roughnessFactor": 0.9}})

    def add_accessor(data, fmt, component_type, acc_type, target, minmax=False):
        while len(binary) % 4:
            binary.append(0)
        offset = len(binary)
        packed = struct.pack("<%d%s" % (len(data), fmt), *data)
        binary.extend(packed)
        buffer_views.append({"buffer": 0, "byteOffset": offset, "byteLength": len(packed), "target": target})
        n = {"VEC3": 3, "SCALAR": 1}[acc_type]
        acc = {"bufferView": len(buffer_views) - 1, "componentType": component_type, "count": len(data) // n, "type": acc_type}
        if minmax:
            acc["min"] = [min(data[i::3]) for i in range(3)]
            acc["max"] = [max(data[i::3]) for i in range(3)]
        accessors.append(acc)
        return len(accessors) - 1

    for i, floor in enumerate(plan.get("floors", [])):
        primitives = []
        for material, mesh in floor_meshes(plan, floor).items():
            if not mesh.indices:
                continue
            primitives.append({
                "attributes": {"POSITION": add_accessor(mesh.positions, "f", 5126, "VEC3", 34962, True),
                               "NORMAL": add_accessor(mesh.normals, "f", 5126, "VEC3", 34962)},
                "indices": add_accessor(mesh.indices, "I", 5125, "SCALAR", 34963),
                "material": material_index[material],
            })
        name = floor.get("name", "floor %d" % (i + 1))
        gltf_meshes.append({"name": name, "primitives": primitives})
        nodes.append({"name": name, "mesh": len(gltf_meshes) - 1})

    gltf = {
        "asset": {"version": "2.0", "generator": "OTNS etc/floorplans/make_glb.py"},
        "scene": 0, "scenes": [{"nodes": list(range(len(nodes)))}], "nodes": nodes, "meshes": gltf_meshes,
        "materials": materials, "accessors": accessors, "bufferViews": buffer_views,
        "buffers": [{"byteLength": len(binary)}],
    }
    json_bytes = json.dumps(gltf, separators=(",", ":")).encode()
    while len(json_bytes) % 4:
        json_bytes += b" "
    while len(binary) % 4:
        binary.append(0)
    total = 12 + 8 + len(json_bytes) + 8 + len(binary)
    with open(path, "wb") as f:
        f.write(struct.pack("<III", 0x46546C67, 2, total))
        f.write(struct.pack("<II", len(json_bytes), 0x4E4F534A) + json_bytes)
        f.write(struct.pack("<II", len(binary), 0x004E4942) + bytes(binary))
    print("%s: %d bytes, %d floors, %d triangles" % (path, total, len(nodes), sum(a["count"] for a in accessors if a["type"] == "SCALAR") // 3))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    write_glb(json.load(open(sys.argv[1])), sys.argv[2])
