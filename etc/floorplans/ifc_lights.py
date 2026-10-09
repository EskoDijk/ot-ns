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
Extract the lighting of one storey of an IFC building model as an OTNS topology: every light
fixture becomes a node at the center of the fixture's geometry, placed on a floor plan made by
ifc2glb.py from the same IFC project (its 'ifcOffset' maps IFC to plan coordinates). Requires the
ifcopenshell package (see ifc2glb.py).

    ./ifc_lights.py <model.ifc> <plan.json> <storey> <out.yaml> <out-lights.json> [--rooms=<pattern>,...] [--name=<plan name>]

    ./ifc_lights.py Clinic_Electrical.ifc clinic.json "Level 1" clinic.yaml clinic-lights.json
    ./ifc_lights.py Clinic_Electrical.ifc clinic_small.json "Level 1" clinic_small.yaml clinic_small-lights.json --rooms="1E*"

Light fixtures are the elements typed by an IfcLightFixtureType (IFC2x3, as exported by Revit)
or of class IfcLightFixture (IFC4). They are classified by their family name (the part of the
element name before the first ':', Revit style): a name with 'switch' is a wall switch, with
'sign' a sign (exit signs, lighted signage), anything else a luminaire; KIND_TYPES gives the OTNS
node type of each kind. Node IDs: the Routers first, then the others, each ordered by room and
position, so that the lights of a room have consecutive IDs.

--rooms selects a part of the storey: the lights (luminaires and signs) in the rooms whose
number matches one of the patterns (shell-style, e.g. '1E*'), the lights in no room that lie
within the area of those (their bounding box plus ROOMLESS_MARGIN), and the switches in the
rooms of the selected lights.

Writes the YAML topology, a JSON file that lists every node with its IFC GlobalId, family, type,
room, electrical panel and circuit (Revit's 'Electrical - Loads' properties, if present) and
position, and sets the plan's 'topology' entry to the YAML file (and its name, with --name).
"""
import fnmatch
import json
import multiprocessing
import os
import sys
import textwrap
from collections import Counter

import ifcopenshell
import ifcopenshell.geom
import ifcopenshell.util.element
import ifcopenshell.util.placement
import ifcopenshell.util.unit

KIND_TYPES = {"luminaire": "router", "sign": "router", "switch": "sed"}
PLURALS = {"luminaire": "luminaires", "sign": "signs", "switch": "wall switches"}
TYPE_NAMES = {"router": "Routers", "fed": "FEDs", "med": "MEDs", "sed": "SEDs", "ssed": "SSEDs"}
ROOMLESS_MARGIN = 1.0  # m, around the selected lights' bounding box, for --rooms


def kind_of(family):
    f = family.lower()
    if "switch" in f:
        return "switch"
    if "sign" in f:
        return "sign"
    return "luminaire"


def storey_of(element):
    parent = element
    while parent is not None and not parent.is_a("IfcBuildingStorey"):
        parent = ifcopenshell.util.element.get_container(parent) or ifcopenshell.util.element.get_aggregate(parent)
    return parent


def light_fixtures(ifc, storey_name):
    out = []
    for element in ifc.by_type("IfcProduct"):
        t = ifcopenshell.util.element.get_type(element)
        if not (element.is_a("IfcLightFixture") or (t is not None and t.is_a("IfcLightFixtureType"))):
            continue
        s = storey_of(element)
        if s is not None and s.Name == storey_name:
            out.append(element)
    return out


def centers(ifc, elements, unit_scale):
    """Element id -> center (meters) of the bounding box of its geometry; the placement point if none."""
    result = {}
    settings = ifcopenshell.geom.settings()
    settings.set("use-world-coords", True)
    iterator = ifcopenshell.geom.iterator(settings, ifc, multiprocessing.cpu_count(), include=elements)
    if iterator.initialize():
        while True:
            shape = iterator.get()
            v = shape.geometry.verts
            result[shape.id] = [(min(v[a::3]) + max(v[a::3])) / 2 * unit_scale for a in range(3)]
            if not iterator.next():
                break
    for element in elements:
        if element.id() not in result:
            m = ifcopenshell.util.placement.get_local_placement(element.ObjectPlacement)
            result[element.id()] = [m[a][3] * unit_scale for a in range(3)]
    return result


def select_rooms(lights, patterns):
    """The lights in the rooms matching the patterns, the lights in no room within their area, and the switches in their rooms."""
    selected = [light for light in lights if light["kind"] != "switch" and any(fnmatch.fnmatchcase(light["room"], p) for p in patterns)]
    if not selected:
        sys.exit("no lights in rooms %s" % ",".join(patterns))
    lo = [min(light["plan"][a] for light in selected) - ROOMLESS_MARGIN for a in range(2)]
    hi = [max(light["plan"][a] for light in selected) + ROOMLESS_MARGIN for a in range(2)]
    selected += [light for light in lights if light["kind"] != "switch" and not light["room"]
                 and all(lo[a] <= light["plan"][a] <= hi[a] for a in range(2))]
    rooms = {light["room"] for light in selected if light["room"]}
    return selected + [light for light in lights if light["kind"] == "switch" and light["room"] in rooms]


def main(ifc_path, plan_path, storey_name, yaml_path, json_path, rooms=None, name=None):
    ifc = ifcopenshell.open(ifc_path)
    unit_scale = ifcopenshell.util.unit.calculate_unit_scale(ifc)
    with open(plan_path) as f:
        plan = json.load(f)
    if name:
        plan["name"] = name
    if "ifcOffset" not in plan:
        sys.exit("%s: no 'ifcOffset'; make the plan with ifc2glb.py from the same IFC project" % plan_path)
    off_x, off_y = plan["ifcOffset"]
    upm = plan.get("unitsPerMeter", 10)
    origin = plan.get("origin", [0, 0])

    elements = light_fixtures(ifc, storey_name)
    if not elements:
        names = ", ".join(s.Name for s in ifc.by_type("IfcBuildingStorey"))
        sys.exit("%s: no light fixtures on storey %r; storeys: %s" % (ifc_path, storey_name, names))
    pos = centers(ifc, elements, unit_scale)

    lights = []
    for element in elements:
        t = ifcopenshell.util.element.get_type(element)
        family = (element.Name or "").split(":")[0]
        kind = kind_of(family)
        room = ifcopenshell.util.element.get_container(element)
        room = room if room is not None and room.is_a("IfcSpace") else None
        loads = ifcopenshell.util.element.get_psets(element).get("Electrical - Loads", {})  # Revit export
        x, y, z = pos[element.id()]
        p = [round(x - off_x, 3), round(off_y - y, 3), round(z, 3)]
        lights.append({"type": KIND_TYPES[kind], "kind": kind, "family": family,
                       "fixtureType": t.Name if t is not None else (element.ObjectType or ""),
                       "room": room.Name if room else "", "roomName": (room.LongName or "") if room else "",
                       "panel": loads.get("Panel", ""), "circuit": str(loads.get("Circuit Number", "")),
                       "guid": element.GlobalId, "plan": p,
                       "otns": [int(round(origin[0] + p[0] * upm)), int(round(origin[1] + p[1] * upm)), int(round(p[2] * upm))]})
    part = "storey '%s'" % storey_name
    if rooms:
        lights = select_rooms(lights, rooms)
        part += " (rooms %s)" % ", ".join(rooms)
    lights.sort(key=lambda light: (light["type"] != "router", light["room"], light["plan"][1], light["plan"][0]))
    for i, light in enumerate(lights, 1):
        light["id"] = i
    for floor in plan.get("floors", []):
        lo, hi = floor.get("elevation", 0), floor.get("elevation", 0) + floor.get("height", 3)
        outside = [light["id"] for light in lights if not lo <= light["plan"][2] <= hi]
        print("plan floor %r (%g..%g m): %d of %d nodes outside" % (floor.get("name"), lo, hi, len(outside), len(lights)))

    header = {"source": os.path.basename(ifc_path), "storey": storey_name, "plan": os.path.basename(plan_path)}
    if rooms:
        header["rooms"] = rooms
    header.update({"unitsPerMeter": upm,
                   "ifcToPlan": "x = X %s %g, y = %g - Y, z = Z (meters)" % ("-" if off_x >= 0 else "+", abs(off_x), off_y)})
    keys = ("id", "type", "kind", "family", "fixtureType", "room", "roomName", "panel", "circuit", "guid", "plan", "otns")
    with open(json_path, "w") as f:  # one line per light
        f.write(json.dumps(header, indent=1)[:-2] + ',\n "lights": [\n')
        f.write(",\n".join("  " + json.dumps({k: light[k] for k in keys}) for light in lights))
        f.write("\n ]\n}\n")

    kinds = Counter(light["kind"] for light in lights)
    types = Counter(light["type"] for light in lights)
    per_type = {}
    for k, n in sorted(kinds.items(), key=lambda kn: -kn[1]):
        per_type.setdefault(KIND_TYPES[k], []).append("%d %s" % (n, PLURALS[k]))
    text = ("OTNS YAML topology of the lighting of %s of the IFC model %s (%s): %s, at the "
            "fixtures' positions on the floor plan %s; generated by ifc_lights.py, see %s for the IFC element "
            "and room of each node (1 unit = %g m)." % (
                part, os.path.basename(ifc_path), plan.get("name", ""),
                ", ".join("%s as %s" % (" and ".join(k), TYPE_NAMES.get(t, t)) for t, k in per_type.items()),
                os.path.basename(plan_path), os.path.basename(json_path), 1 / upm))
    lines = ["#"] + ["# " + line for line in textwrap.wrap(text, 98)] + ["#", "network:", "    pos-shift: [0, 0, 0]", "nodes:"]
    for light in lights:
        lines += ["    - id: %d" % light["id"], "      type: %s" % light["type"], "      pos: [%d, %d, %d]" % tuple(light["otns"])]
    with open(yaml_path, "w") as f:
        f.write("\n".join(lines) + "\n")

    plan["topology"] = os.path.relpath(yaml_path, os.path.dirname(os.path.abspath(plan_path)))
    with open(plan_path, "w") as f:
        json.dump(plan, f, indent=2)
        f.write("\n")
    print("%d nodes: %s; %s" % (len(lights), ", ".join("%d %s" % (n, k) for k, n in kinds.items()),
                                ", ".join("%d %s" % (n, t) for t, n in types.items())))
    print("fixture types:", ", ".join("%d %s" % (n, t) for t, n in Counter(light["fixtureType"] for light in lights).most_common()))


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    opts = dict(a[2:].split("=", 1) for a in sys.argv[1:] if a.startswith("--") and "=" in a)
    if len(args) != 5:
        sys.exit(__doc__)
    main(*args, rooms=opts["rooms"].split(",") if opts.get("rooms") else None, name=opts.get("name"))
