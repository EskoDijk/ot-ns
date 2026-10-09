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
Convert an IFC building model to a glTF model (.glb) and a floor plan JSON for the 3D web
visualization skin of OTNS (see README.md). Requires the ifcopenshell package:

    pip install ifcopenshell
    ./ifc2glb.py <model.ifc> [<more.ifc>[:IfcType,...] ...] <out.glb> [<plan.json>]
                 [--no-openings] [--storey=<name>] [--name=<plan name>]

    ./ifc2glb.py AC20-Institute-Var-2.ifc institute.glb institute.json
    ./ifc2glb.py Clinic_Architectural.ifc Clinic_Structural.ifc:IfcSlab clinic.glb clinic.json --storey="First Floor"

The first IFC file is the main model: its storeys become the plan's floors and its walls the
plan's walls. More IFC files of the same building (e.g. the structural model of a project split
per discipline) add their elements to the storey of the main model with the same name or
elevation; ':IfcType,...' after a file name converts only those element types of that file.
--storey converts only one storey, which becomes the plan's only floor.

The IFC building is placed with its bounding box corner at plan (0, 0); IFC's z-up coordinates
become the plan's coordinates with the plan's y axis pointing to IFC south (so that a top view
shows north up, as on a drawing). The plan's 'ifcOffset' records this: IFC point (X, Y, Z)
meters is at plan (X - ifcOffset[0], ifcOffset[1] - Y) at height Z, for scripts that place
nodes from the same IFC project (e.g. clinic_lights.py). One glTF node per storey, named after
the storey, so that the visualizer can hide floors. Materials: the IFC surface colors when
present, with a translucency per element type so that nodes inside the building stay visible.
Windows and doors are included unless --no-openings is given (walls keep their openings either
way).
"""
import json
import multiprocessing
import re
import struct
import sys

import ifcopenshell
import ifcopenshell.geom
import ifcopenshell.util.element
import ifcopenshell.util.placement
import ifcopenshell.util.unit

# element types to convert, with the fallback color (r, g, b) and the alpha applied to all
ELEMENT_TYPES = {
    "IfcWall": ((0.87, 0.80, 0.68), 0.6),
    "IfcSlab": ((0.78, 0.80, 0.82), 0.9),
    "IfcRoof": ((0.60, 0.35, 0.30), 0.6),
    "IfcStair": ((0.65, 0.65, 0.65), 0.9),
    "IfcColumn": ((0.70, 0.70, 0.72), 0.9),
    "IfcBeam": ((0.70, 0.70, 0.72), 0.9),
    "IfcRailing": ((0.50, 0.50, 0.55), 0.8),
    "IfcCurtainWall": ((0.60, 0.75, 0.85), 0.4),
    "IfcPlate": ((0.60, 0.75, 0.85), 0.4),
    "IfcMember": ((0.60, 0.60, 0.65), 0.8),
    "IfcWindow": ((0.55, 0.75, 0.90), 0.35),
    "IfcDoor": ((0.55, 0.40, 0.25), 0.8),
}
OPENING_TYPES = ("IfcWindow", "IfcDoor")
DEFAULT_STOREY_HEIGHT = 3.0


def element_type_of(element):
    for t in ELEMENT_TYPES:
        if element.is_a(t):
            return t
    return None


def storey_of(element):
    """The IfcBuildingStorey of an element, also via a space or a whole (a curtain wall's plates); None if none."""
    parent = element
    while parent is not None and not parent.is_a("IfcBuildingStorey"):
        parent = ifcopenshell.util.element.get_container(parent) or ifcopenshell.util.element.get_aggregate(parent)
    return parent


def main(inputs, glb_path, plan_path=None, openings=True, storey_name=None, name=None):
    files = []  # (path, ifc, element types)
    for spec in inputs:
        path, types = re.match(r"(?i)(.*?\.ifc)(?::(.*))?$", spec).groups()
        types = types.split(",") if types else [t for t in ELEMENT_TYPES if openings or t not in OPENING_TYPES]
        files.append((path, ifcopenshell.open(path), types))
    ifc_path, ifc = files[0][0], files[0][1]
    unit_scale = ifcopenshell.util.unit.calculate_unit_scale(ifc)  # model length unit -> meters

    storeys = sorted(ifc.by_type("IfcBuildingStorey"), key=lambda s: (s.Elevation or 0))
    storey_names = [s.Name or s.LongName or ("storey %d" % (i + 1)) for i, s in enumerate(storeys)]
    selected = None
    if storey_name is not None:
        if storey_name not in storey_names:
            sys.exit("%s: no storey %r; storeys: %s" % (ifc_path, storey_name, ", ".join(storey_names)))
        selected = storey_names.index(storey_name)

    # per storey: {material key: (color, alpha, positions[], indices[])}
    per_storey = [{} for _ in storeys] + [{}]  # last: elements without a storey
    bounds = [float("inf")] * 3 + [float("-inf")] * 3
    for path, f, types in files:
        scale = ifcopenshell.util.unit.calculate_unit_scale(f)
        # the storeys of this file -> index of the main model's storey with the same name or elevation
        storey_index = {}
        for s in f.by_type("IfcBuildingStorey"):
            for i, ms in enumerate(storeys):
                if s.Name == ms.Name or abs((s.Elevation or 0) * scale - (ms.Elevation or 0) * unit_scale) < 0.01:
                    storey_index[s.id()] = i
                    break
        elements = []
        for t in types:
            for element in f.by_type(t):
                if element_type_of(element) is None:
                    continue
                s = storey_of(element)
                si = storey_index.get(s.id(), len(storeys)) if s is not None else len(storeys)
                if selected is None or si == selected:
                    elements.append((element, si))
        count = convert(f, scale, elements, per_storey, bounds)
        print("%s: converted %d of %d elements of types %s, unit scale %g" % (path, count, len(elements), ", ".join(types), scale))
    print("IFC bounds x %.1f..%.1f y %.1f..%.1f z %.1f..%.1f m" % (bounds[0], bounds[3], bounds[1], bounds[4], bounds[2], bounds[5]))

    # IFC (x, y, z) z-up -> glTF (x - minx, z - minz_ground, maxy - y) y-up, plan y towards IFC south
    min_x, max_y = bounds[0], bounds[4]
    ground = 0.0
    def to_gltf(x, y, z):
        return x - min_x, z - ground, max_y - y

    write_glb(glb_path, per_storey, storey_names + [""], to_gltf)

    if plan_path:
        floors = []
        for i, s in enumerate(storeys):
            if selected is not None and i != selected:
                continue
            elevation = (s.Elevation or 0) * unit_scale
            if i + 1 < len(storeys):
                height = (storeys[i + 1].Elevation or 0) * unit_scale - elevation
            else:
                height = DEFAULT_STOREY_HEIGHT
            floors.append({"name": storey_names[i], "elevation": round(elevation, 3) + 0.0, "height": round(height, 3),
                           "walls": wall_axes(ifc, s, unit_scale, to_gltf)})
        plan = {
            "name": name or "%s (from %s)" % (ifc.by_type("IfcProject")[0].Name or "IFC building", ifc_path.split("/")[-1]),
            "unitsPerMeter": 10, "origin": [100, 100], "nodeScale": 0.25,
            "model": {"url": glb_path.split("/")[-1], "position": [0, 0, 0], "rotation": 0, "scale": 1},
            "ifcOffset": [round(min_x, 4), round(max_y, 4)],
            "floors": floors,
        }
        with open(plan_path, "w") as f:
            json.dump(plan, f, indent=2)
            f.write("\n")
        print("%s: %d floors: %s" % (plan_path, len(floors), ", ".join("%s @ %g m" % (f["name"], f["elevation"]) for f in floors)))


def convert(ifc, unit_scale, elements, per_storey, bounds):
    """Triangulate (element, storey index) pairs into the per-storey material buckets."""
    if not elements:
        return 0
    storey_of_id = {e.id(): si for e, si in elements}
    settings = ifcopenshell.geom.settings()
    settings.set("use-world-coords", True)
    iterator = ifcopenshell.geom.iterator(settings, ifc, multiprocessing.cpu_count(), include=[e for e, _ in elements])
    count = 0
    if iterator.initialize():
        while True:
            shape = iterator.get()
            element = ifc.by_id(shape.id)
            etype = element_type_of(element)
            verts = shape.geometry.verts
            faces = shape.geometry.faces
            mat_ids = shape.geometry.material_ids
            materials = shape.geometry.materials
            fallback_color, alpha = ELEMENT_TYPES[etype]
            buckets = per_storey[storey_of_id[shape.id]]
            for f in range(len(faces) // 3):
                mid = mat_ids[f] if f < len(mat_ids) else -1
                color = fallback_color
                if 0 <= mid < len(materials):
                    try:
                        d = materials[mid].diffuse
                        color = (round(d.r(), 3), round(d.g(), 3), round(d.b(), 3))
                    except (AttributeError, RuntimeError):
                        pass  # style without a diffuse color: keep the fallback
                key = (etype, color)
                if key not in buckets:
                    buckets[key] = [color, alpha, [], []]
                positions, indices = buckets[key][2], buckets[key][3]
                for k in range(3):
                    v = faces[f * 3 + k] * 3
                    x, y, z = verts[v] * unit_scale, verts[v + 1] * unit_scale, verts[v + 2] * unit_scale
                    for a, c in enumerate((x, y, z)):
                        bounds[a] = min(bounds[a], c)
                        bounds[a + 3] = max(bounds[a + 3], c)
                    indices.append(len(positions) // 3)
                    positions += [x, y, z]
            count += 1
            if not iterator.next():
                break
    return count


def wall_axes(ifc, storey, unit_scale, to_gltf):
    """The walls of a storey as plan segments (meters), from the IfcWall 'Axis' representations."""
    walls = []
    for wall in ifc.by_type("IfcWall"):
        if ifcopenshell.util.element.get_container(wall) != storey:
            continue
        try:
            axis = next((r for r in wall.Representation.Representations if r.RepresentationIdentifier == "Axis"), None)
            if axis is None or len(axis.Items) != 1 or not axis.Items[0].is_a("IfcPolyline"):
                continue
            m = ifcopenshell.util.placement.get_local_placement(wall.ObjectPlacement)
            pts = []
            for p in axis.Items[0].Points:
                c = list(p.Coordinates) + [0.0, 0.0]
                x = (m[0][0] * c[0] + m[0][1] * c[1] + m[0][3]) * unit_scale
                y = (m[1][0] * c[0] + m[1][1] * c[1] + m[1][3]) * unit_scale
                gx, _, gz = to_gltf(x, y, 0)
                pts.append([round(gx, 3), round(gz, 3)])
            for a, b in zip(pts, pts[1:]):
                walls.append({"from": a, "to": b})
        except Exception as err:  # noqa: BLE001 - best effort per wall
            print("wall %s: no axis (%s)" % (wall.GlobalId, err))
    return walls


def write_glb(path, per_storey, node_names, to_gltf):
    binary = bytearray()
    buffer_views, accessors, materials, meshes, nodes = [], [], [], [], []
    material_index = {}
    triangles = 0

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

    for si, buckets in enumerate(per_storey):
        primitives = []
        for (etype, color), (_, alpha, positions, indices) in buckets.items():
            if not indices:
                continue
            mkey = (etype, color, alpha)
            if mkey not in material_index:
                material_index[mkey] = len(materials)
                materials.append({"name": "%s %s" % (etype, color), "doubleSided": True,
                                  "alphaMode": "BLEND" if alpha < 1 else "OPAQUE",
                                  "pbrMetallicRoughness": {"baseColorFactor": list(color) + [alpha], "metallicFactor": 0.0, "roughnessFactor": 0.9}})
            gpos = []
            for i in range(0, len(positions), 3):
                gpos += to_gltf(positions[i], positions[i + 1], positions[i + 2])
            # no normals: the loader then uses flat shading, which suits a building
            primitives.append({"attributes": {"POSITION": add_accessor(gpos, "f", 5126, "VEC3", 34962, True)},
                               "indices": add_accessor(indices, "I", 5125, "SCALAR", 34963),
                               "material": material_index[mkey]})
            triangles += len(indices) // 3
        if not primitives:
            continue
        name = node_names[si] or "no storey"
        meshes.append({"name": name, "primitives": primitives})
        nodes.append({"name": name, "mesh": len(meshes) - 1})

    gltf = {"asset": {"version": "2.0", "generator": "OTNS etc/floorplans/ifc2glb.py"},
            "scene": 0, "scenes": [{"nodes": list(range(len(nodes)))}], "nodes": nodes, "meshes": meshes,
            "materials": materials, "accessors": accessors, "bufferViews": buffer_views,
            "buffers": [{"byteLength": len(binary)}]}
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
    print("%s: %.1f MB, %d nodes, %d materials, %d triangles" % (path, total / 1e6, len(nodes), len(materials), triangles))


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    opts = dict(a[2:].split("=", 1) for a in sys.argv[1:] if a.startswith("--") and "=" in a)
    inputs = [a for a in args if re.match(r"(?i).*\.ifc(:.*)?$", a)]
    outputs = args[len(inputs):]
    if not inputs or args[:len(inputs)] != inputs or not 1 <= len(outputs) <= 2:
        sys.exit(__doc__)
    main(inputs, outputs[0], outputs[1] if len(outputs) > 1 else None, openings="--no-openings" not in sys.argv,
         storey_name=opts.get("storey"), name=opts.get("name"))
