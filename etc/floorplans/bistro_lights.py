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
"""
Extract the luminaires of the Godot "Bistro Demo Tweaked" scene (github.com/Jamsers/Bistro-Demo-Tweaked)
as an OTNS topology: every light becomes a node with the light's position, so that the OTNS
simulation and the Godot scene share one set of luminaire identities and coordinates.

    ./bistro_lights.py <path to Bistro-Demo-Tweaked> bistro.yaml bistro-lights.json

Luminaires found in the scene:
  - the Light3D nodes of the night-light groups of MainScene.tscn: "Night Lights/StreetLamps"
    (street lamps), "Building_Lamps" (wall lamps on the houses), "Sign_Lights", and the
    spotlights of "Bistro Spotlights", "Building Spotlights" and "PatchLight";
  - hanging lanterns: the Lantern_Wind_* roots of Scenes/Section02/Bistro_Lanterns.tscn
    (each carries an OmniLight3D);
  - string lights: Meshes/Section02/Bistro_StringLights.glb has one node per string
    (StringLight_Wind_*) with sub-meshes _1.._5 of colored bulbs; each connected component of
    such a sub-mesh is one bulb, its color comes from the material override in
    Scenes/Section02/Bistro_StringLights.tscn.
Coordinates: Godot (x right, y up, z towards the viewer, meters) -> OTNS units:
  x = (x_g + X_OFFSET) * UNITS_PER_METER, y = (z_g + Z_OFFSET) * UNITS_PER_METER, z = y_g * UNITS_PER_METER.
The JSON output lists every light with its Godot node path, kind, color and position, for a
Godot client that maps OTNS node IDs back to scene nodes.
"""
import json
import os
import re
import struct
import sys
from collections import defaultdict

UNITS_PER_METER = 10
X_OFFSET = 60.0   # m, so that all OTNS coordinates are positive
Z_OFFSET = 80.0   # m


def transform_origin(line):
    """The translation of a 'transform = Transform3D(...)' line."""
    nums = [float(x) for x in re.search(r"Transform3D\((.*)\)", line).group(1).split(",")]
    return nums[9], nums[10], nums[11]


# night-light groups of MainScene.tscn -> (kind, OTNS node type)
LIGHT_GROUPS = {
    "StreetLamps": ("streetlamp", "router"),
    "Building_Lamps": ("walllamp", "router"),
    "Sign_Lights": ("signlight", "router"),
    "Bistro Spotlights": ("spotlight", "fed"),
    "Building Spotlights": ("spotlight", "fed"),
    "PatchLight": ("spotlight", "fed"),
}


def night_lights(demo):
    """Light3D nodes of the night-light groups: (name, kind, node type, position)."""
    lights = []
    lines = open(os.path.join(demo, "MainScene.tscn")).read().split("\n")
    for i, line in enumerate(lines):
        m = re.match(r'\[node name="([^"]+)" type="(OmniLight3D|SpotLight3D)" parent="Night Lights/([^"/]+)"\]', line)
        if m and m.group(3) in LIGHT_GROUPS:
            kind, node_type = LIGHT_GROUPS[m.group(3)]
            for l2 in lines[i + 1:i + 8]:
                if l2.startswith("transform"):
                    lights.append(("Night Lights/%s/%s" % (m.group(3), m.group(1)), kind, node_type, transform_origin(l2)))
                    break
    return lights


def lanterns(demo):
    """Lantern_Wind_* roots of the lanterns scene: (name, position)."""
    out = []
    lines = open(os.path.join(demo, "Scenes/Section02/Bistro_Lanterns.tscn")).read().split("\n")
    for i, line in enumerate(lines):
        m = re.match(r'\[node name="(Lantern_Wind_\d+)" type="Node3D" parent="\."\]', line)
        if m and lines[i + 1].startswith("transform"):
            out.append(("Props/Section02/Bistro_Lanterns/" + m.group(1), transform_origin(lines[i + 1])))
    return out


def bulb_colors(demo):
    """(string node name, sub-mesh index) -> color name, from the material overrides."""
    text = open(os.path.join(demo, "Scenes/Section02/Bistro_StringLights.tscn")).read()
    ids = {m.group(2): m.group(1) for m in re.finditer(r'path="res://Materials/Lights/StringLight_(\w+)\.tres" id="([^"]+)"', text)}
    colors = {}
    pattern = r'\[node name="(StringLight_Wind_\d+)_(\d+)" parent="[^"]+" index="\d+"\]\n(?:[^\[\n]*\n)*?surface_material_override/0 = ExtResource\("([^"]+)"\)'
    for m in re.finditer(pattern, text):
        if m.group(3) in ids:
            colors[(m.group(1), int(m.group(2)))] = ids[m.group(3)].lower()
    return colors


def load_glb(path):
    b = open(path, "rb").read()
    total = struct.unpack_from("<III", b, 0)[2]
    off, js, bin_ = 12, None, None
    while off < total:
        ln, typ = struct.unpack_from("<II", b, off)
        off += 8
        chunk = b[off:off + ln]
        off += ln
        if typ == 0x4E4F534A:
            js = json.loads(chunk)
        elif typ == 0x004E4942:
            bin_ = chunk
    return js, bin_


def accessor(js, bin_, idx):
    a = js["accessors"][idx]
    bv = js["bufferViews"][a["bufferView"]]
    fmt, size = {5126: ("f", 4), 5125: ("I", 4), 5123: ("H", 2), 5121: ("B", 1)}[a["componentType"]]
    n = {"SCALAR": 1, "VEC2": 2, "VEC3": 3, "VEC4": 4}[a["type"]]
    start = bv.get("byteOffset", 0) + a.get("byteOffset", 0)
    stride = bv.get("byteStride", size * n)
    return [struct.unpack_from("<%d%s" % (n, fmt), bin_, start + i * stride) for i in range(a["count"])]


def mat_mul(a, b):
    return [[sum(a[i][k] * b[k][j] for k in range(4)) for j in range(4)] for i in range(4)]


def node_matrix(nd):
    if "matrix" in nd:
        m = nd["matrix"]
        return [[m[c * 4 + r] for c in range(4)] for r in range(4)]  # column-major -> rows
    t = nd.get("translation", [0, 0, 0])
    x, y, z, w = nd.get("rotation", [0, 0, 0, 1])
    s = nd.get("scale", [1, 1, 1])
    r = [[1 - 2 * (y * y + z * z), 2 * (x * y - z * w), 2 * (x * z + y * w)],
         [2 * (x * y + z * w), 1 - 2 * (x * x + z * z), 2 * (y * z - x * w)],
         [2 * (x * z - y * w), 2 * (y * z + x * w), 1 - 2 * (x * x + y * y)]]
    return [[r[i][j] * s[j] for j in range(3)] + [t[i]] for i in range(3)] + [[0, 0, 0, 1]]


def world_matrix(js, idx, parents):
    chain = []
    while idx is not None:
        chain.append(idx)
        idx = parents.get(idx)
    m = [[float(i == j) for j in range(4)] for i in range(4)]
    for i in reversed(chain):
        m = mat_mul(m, node_matrix(js["nodes"][i]))
    return m


def components(positions, indices):
    """Connected components of a triangle mesh (vertices at equal positions are merged)."""
    key, canon = {}, []
    for p in positions:
        canon.append(key.setdefault((round(p[0], 4), round(p[1], 4), round(p[2], 4)), len(key)))
    parent = list(range(len(key)))

    def find(a):
        while parent[a] != a:
            parent[a] = parent[parent[a]]
            a = parent[a]
        return a

    for t in range(0, len(indices), 3):
        a = find(canon[indices[t][0]])
        for k in (1, 2):
            parent[find(canon[indices[t + k][0]])] = a
    groups = defaultdict(list)
    for i, p in enumerate(positions):
        groups[find(canon[i])].append(p)
    return list(groups.values())


def string_bulbs(demo):
    """(name, color, position) per bulb of the string lights."""
    js, bin_ = load_glb(os.path.join(demo, "Meshes/Section02/Bistro_StringLights.glb"))
    colors = bulb_colors(demo)
    parents = {c: i for i, nd in enumerate(js["nodes"]) for c in nd.get("children", [])}
    bulbs = []
    for i, nd in enumerate(js["nodes"]):
        m = re.match(r"(StringLight_Wind_\d+)\.(\d+)$", nd.get("name", ""))
        if not m or "mesh" not in nd:
            continue
        string, sub = m.group(1), int(m.group(2))
        if sub == 0:
            continue  # the wire and the bulb sockets
        color = colors.get((string, sub), "white")
        wm = world_matrix(js, i, parents)
        for prim in js["meshes"][nd["mesh"]]["primitives"]:
            pos = accessor(js, bin_, prim["attributes"]["POSITION"])
            idx = accessor(js, bin_, prim["indices"])
            for k, comp in enumerate(sorted(components(pos, idx), key=lambda c: (sum(p[0] for p in c), sum(p[2] for p in c)))):
                c = [sum(p[a] for p in comp) / len(comp) for a in range(3)]
                w = [sum(wm[r][a] * c[a] for a in range(3)) + wm[r][3] for r in range(3)]
                bulbs.append(("Props/Section02/Bistro_StringLights/%s/%s_%d#%d" % (string, string, sub, k), color, tuple(w)))
    return bulbs


def to_otns(p):
    return [int(round((p[0] + X_OFFSET) * UNITS_PER_METER)), int(round((p[2] + Z_OFFSET) * UNITS_PER_METER)), int(round(p[1] * UNITS_PER_METER))]


def main(demo, yaml_path, json_path):
    lights = []
    for name, kind, node_type, p in night_lights(demo):
        lights.append({"name": name, "kind": kind, "type": node_type, "color": "warm white", "godot": [round(v, 3) for v in p]})
    for name, p in lanterns(demo):
        lights.append({"name": name, "kind": "lantern", "type": "fed", "color": "orange", "godot": [round(v, 3) for v in p]})
    for name, color, p in string_bulbs(demo):
        lights.append({"name": name, "kind": "stringlight", "type": "sed", "color": color, "godot": [round(v, 3) for v in p]})
    for i, light in enumerate(lights, 1):
        light["id"] = i
        light["otns"] = to_otns(light["godot"])
    with open(json_path, "w") as f:
        json.dump({"source": "Bistro-Demo-Tweaked", "unitsPerMeter": UNITS_PER_METER,
                   "godotToOtns": "x = (x_g + %g) * u, y = (z_g + %g) * u, z = y_g * u" % (X_OFFSET, Z_OFFSET),
                   "lights": lights}, f, indent=1)
        f.write("\n")
    lines = ["#", "# OTNS YAML topology of the luminaires of the Godot 'Bistro Demo Tweaked' scene: street, wall and",
             "# sign lamps as Routers, spotlights and hanging lanterns as FEDs, string light bulbs as SEDs;",
             "# generated by bistro_lights.py, see bistro-lights.json for the Godot node of each OTNS node",
             "# (1 unit = 0.1 m).", "#",
             "network:", "    pos-shift: [0, 0, 0]", "nodes:"]
    for light in lights:
        lines += ["    - id: %d" % light["id"], "      type: %s" % light["type"],
                  "      pos: [%d, %d, %d]" % tuple(light["otns"])]
    open(yaml_path, "w").write("\n".join(lines) + "\n")
    kinds = defaultdict(int)
    for light in lights:
        kinds[light["kind"]] += 1
    print("%d lights: %s" % (len(lights), ", ".join("%d %s" % (n, k) for k, n in kinds.items())))
    colors = defaultdict(int)
    for light in lights:
        if light["kind"] == "stringlight":
            colors[light["color"]] += 1
    print("string light colors:", dict(colors))


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(*sys.argv[1:])
