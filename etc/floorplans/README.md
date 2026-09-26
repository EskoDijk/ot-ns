# Floor plans for the 3D web visualization

A floor plan describes a building that the `office3d` skin of the web visualizer (preset
`space`, see the [GUIDE](../../GUIDE.md#network-visualization-skins)) draws around the nodes.
Start OTNS with `-floorplan <file.json>`; the visualizer fetches the file when the 3D skin is
activated, so the file may be edited while OTNS runs and a page reload shows the result.

The building is drawn as floor slabs and translucent walls. It has no effect on the simulation:
the radio model does not (yet) know about walls.

## Format

JSON, with all building dimensions in meters:

```json
{
  "name": "Small office, two floors",
  "topology": "office-small.yaml",
  "unitsPerMeter": 10,
  "origin": [100, 100],
  "nodeScale": 0.25,
  "wallThickness": 0.2,
  "doorHeight": 2.1,
  "floors": [
    {
      "name": "Ground floor",
      "elevation": 0.0,
      "height": 3.0,
      "outline": [40, 16],
      "walls": [
        {"from": [0, 7], "to": [40, 7], "openings": [{"at": 4, "width": 1.0}]},
        {"from": [8, 0], "to": [8, 7]}
      ]
    }
  ]
}
```

| Key | Meaning |
|---|---|
| `topology` | Optional YAML topology file (the format of the `load` command, see `../mesh-topologies`), relative to the plan file, that OTNS loads at startup when started with `-floorplan`. |
| `unitsPerMeter` | OTNS position units per meter. The default radio model uses 0.1 m per unit, i.e. 10 units per meter (`radioparam MeterPerUnit`). |
| `origin` | OTNS `[x, y]` position of the building's `(0, 0)` corner. |
| `nodeScale` | Scale factor for the node shapes in the 3D view (default 1), so that nodes look right at the building's scale: 0.25 makes a Router prism 4 units, i.e. 0.4 m, wide. |
| `wallThickness`, `doorHeight` | Defaults for all walls and openings, in meters (0.2 and 2.1). |
| `floors[].elevation`, `height` | Height of the floor slab's top above OTNS `z = 0`, and the height of the walls. |
| `floors[].outline` | `[width, depth]` of the rectangular footprint from `(0, 0)`. It gives the floor slab and the four exterior walls. |
| `floors[].walls` | Interior walls: segments `from`/`to` in meters, an optional `thickness`, and optional `openings` (doors): each a gap of `width` centered at distance `at` along the wall from its `from` point, with a lintel above `doorHeight`. |

### glTF model for the looks

Instead of the procedural slabs and walls, a plan can show a glTF model of the building:

```json
  "model": {"url": "office-small.glb", "position": [0, 0, 0], "rotation": 0, "scale": 1}
```

| Key | Meaning |
|---|---|
| `url` | The model file (`.glb` or `.gltf`), relative to the plan file's directory, which OTNS serves under `/floorplan/`; an absolute URL also works. |
| `position` | `[x, y, z]` meters: where the model's origin lies on the plan (`z` up). Default `[0, 0, 0]`. |
| `rotation` | Degrees, clockwise seen from above, about the model's origin. Default 0. |
| `scale` | Extra scale factor if the model is not in meters. Default 1. |

The model is expected in meters with y up and its z axis along the plan's y axis, which is the
glTF convention for a model exported from a plan-like drawing. With a model, the plan's own slabs
and walls are hidden; key `w` shows them over the model to check the alignment. The plan's floors
are still used for the height of the nodes' floors and for hiding floors: a top-level model node
named like a plan floor (ignoring case, spaces and punctuation) is hidden with that floor.
Translucent materials of the model (glTF `BLEND`) are drawn without depth writes, like the plan's
walls, so that they don't hide each other depending on the camera angle.

### From an IFC building model

`ifc2glb.py` converts an IFC (BIM) file to a model and a matching plan, using the
[ifcopenshell](https://ifcopenshell.org) Python package:

```
pip install ifcopenshell
./ifc2glb.py building.ifc building.glb building.json [--no-openings]
```

It converts walls, slabs, roofs, stairs, columns, beams, railings, curtain walls, windows and
doors (`--no-openings` leaves out windows and doors; walls keep their openings either way) with
the IFC surface colors and a translucency per element type, one glTF node per storey named after
it, and writes a plan with the storeys as floors (elevation and height from the IFC) and the walls'
axis lines as plan walls, ready for a future wall-aware radio model. The building is placed with
its bounding box corner at plan `(0, 0)`, seen from above with IFC north up.

Other routes: IfcOpenShell's `IfcConvert` command (to OBJ or glTF) followed by Blender's glTF
export, or assembling a building from a CC0 kit such as Kenney's Building Kit. Check the license
of any model before committing it; CC0 and CC-BY are fine, NoDerivatives is not.

`make_glb.py <plan.json> <model.glb>` writes a test model from a plan (the same slabs, walls and
doors as boxes, one node per floor), used for `office-small.glb`.

Node positions are OTNS units: a point `(x, y)` meters on floor `f` is at
`[origin.x + x * unitsPerMeter, origin.y + y * unitsPerMeter, (f.elevation + h) * unitsPerMeter]`
for a height `h` above that floor.

## Files

- `office-small.json`: a 40 x 16 m office on two floors, a corridor along the middle with
  offices on both sides and a 16 m meeting room on the ground floor. `office-small-model.json`
  is the same plan shown with the glTF model `office-small.glb`. Both load the topology
  `office-small.yaml` at startup: 30 Routers as ceiling luminaires. Run:

  ```
  otns -floorplan etc/floorplans/office-small.json
  > cv skin space
  ```

- `institute.json` and `institute.glb`: a real four-storey office building (44 x 19 m, with a
  basement, a stair tower and a roof) converted with `ifc2glb.py` from the IFC 4 example
  `AC20-Institute-Var-2.ifc` of the Institute for Automation and Applied Informatics (IAI),
  Karlsruhe Institute of Technology (KIT), available for unrestricted use from the
  [KIT IFC examples](https://www.ifcwiki.org/index.php?title=KIT_IFC_Examples); KIT asks that
  publications using it cite that source. Its topology `institute.yaml`, loaded at startup, has
  18 Routers under the ceilings of the three main storeys:

  ```
  otns -floorplan etc/floorplans/institute.json
  > cv skin space
  ```

In the 3D view, keys `1`..`9` toggle the visibility of a floor (from the bottom), `0` shows all,
and `g` enters walk mode: first-person navigation inside the building (W/A/S/D or arrows, mouse
look, Shift runs), drawn opaque with ceilings; the plan's slabs and walls or the model's meshes are
the collision geometry, so walls block and modelled stairs can be climbed. A plan floor without a
floor above gets a ceiling in walk mode.
