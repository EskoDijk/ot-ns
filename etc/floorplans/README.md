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
| `unitsPerMeter` | OTNS position units per meter. The default radio model uses 0.1 m per unit, i.e. 10 units per meter (`radioparam MeterPerUnit`). |
| `origin` | OTNS `[x, y]` position of the building's `(0, 0)` corner. |
| `nodeScale` | Scale factor for the node shapes in the 3D view (default 1), so that nodes look right at the building's scale: 0.25 makes a Router prism 4 units, i.e. 0.4 m, wide. |
| `wallThickness`, `doorHeight` | Defaults for all walls and openings, in meters (0.2 and 2.1). |
| `floors[].elevation`, `height` | Height of the floor slab's top above OTNS `z = 0`, and the height of the walls. |
| `floors[].outline` | `[width, depth]` of the rectangular footprint from `(0, 0)`. It gives the floor slab and the four exterior walls. |
| `floors[].walls` | Interior walls: segments `from`/`to` in meters, an optional `thickness`, and optional `openings` (doors): each a gap of `width` centered at distance `at` along the wall from its `from` point, with a lintel above `doorHeight`. |

Node positions are OTNS units: a point `(x, y)` meters on floor `f` is at
`[origin.x + x * unitsPerMeter, origin.y + y * unitsPerMeter, (f.elevation + h) * unitsPerMeter]`
for a height `h` above that floor.

## Files

- `office-small.json`: a 40 x 16 m office on two floors, a corridor along the middle with
  offices on both sides and a 16 m meeting room on the ground floor. Companion topology:
  `../mesh-topologies/office-small-3d.yaml` with 30 Routers as ceiling luminaires. Run:

  ```
  otns -floorplan etc/floorplans/office-small.json
  > load "etc/mesh-topologies/office-small-3d.yaml"
  > cv skin space
  ```

In the 3D view, keys `1`..`9` toggle the visibility of a floor (from the bottom), `0` shows all.
