# 3D controller models

TuiSense draws every controller as a real triangle mesh with Limoni's
`widgets.Viewer3D` (depth-tested, back-face culled, Lambert/Gouraud lit). The
meshes are generated procedurally from data-only descriptors, cached, and
shared by rendering, live-input animation and mouse picking.

This document covers the architecture, the ten built-in models, their accuracy
and licensing, and how to add a new model.

## Architecture

```
internal/ui/geom/      mesh primitives (no TuiSense types)
  geom.go              Xform, Builder: Box, Cylinder, Frustum, Prism, Slab, Ellipsoid
  outline.go           outlines: Mirror, Smooth (Catmull-Rom), Inset, ear-clipping Triangulate

internal/ui/visual/    the model system
  spec.go              ModelSpec, Body, Component, Geometry, kinds, surfaces, verification
  parts.go             reusable component builders: stick, dpad, face buttons, shoulders, grips…
  build.go             generator: ModelSpec → cached mesh (millimetres → renderer units)
  model.go             Model.Frame: pose animated parts + colour faces, zero allocations
  camera.go            Camera, FitScale (bounding-box framing), Pick, ScreenPos
  preview.go           RenderPreview: offline PNG rasteriser for contributors
  registry.go          Register / Lookup / Gallery
  models_*.go          the specs

internal/state/stage.go        camera presets & transitions, component selection, gallery state
internal/ui/widgets/controller3d.go   the stage: Viewer3D + pointer handling + tooltip
internal/ui/views/gallery.go   the Controller Gallery inspector
```

### Coordinates

Specs are written in millimetres. +X is the player's right, +Y is up and −Z is
the front face (the camera looks from −Z towards +Z). Faces wind
counter-clockwise seen from outside; `geom` tests check every primitive.

### Body

`Body.Half` is the right half of the front silhouette, from the top centre
clockwise to the bottom centre. It is mirrored, spline-smoothed (`Smooth`
points per edge; `1` keeps hard facets for angular designs), ear-clipped and
extruded into a bevelled slab. A rear "spine" carries the triggers; grip
bulges are ellipsoid components on the `Inside` surface.

### Components

Every interactive part is a `Component`:

| Field | Meaning |
| --- | --- |
| `ID` | stable identifier (`paddle_p1`, `back_left`, …) |
| `Kind` | `left_stick`, `face_button`, `rear_paddle`, `touchpad`, `lighting_zone`, … — models may `RegisterKind` their own (`battery_compartment`, `nfc_reader`) |
| `Label` | the manufacturer's label where verified (`P1`, `L4`, `Fn (left)`), otherwise a neutral description |
| `Surface` / `Side` | where it is mounted; geometry is placed on that surface automatically |
| `Shapes` | one or more `Geometry` primitives |
| `Button` + `Bound` | the physical input event; it only lights up when the active driver's `InputLayout` declares it |
| `Region` | the capability region (and settings panel / diagnostic test) it opens on connected hardware |
| `Anim` | stick tilt or trigger pull driven by telemetry |
| `Verify` + `Note` | `verified`, `approximate` or `unverified`, with the source or caveat |

`KindDecor` geometry (stick rings, D-pad dishes, trim) is drawn but never picked.

### Picking

`Model.Pick` projects the current pose with exactly the rotation order,
distance and scale `Viewer3D` uses, then tests the cell's sample dots against
every front-facing triangle; the nearest hit wins. Selection therefore follows
rotation, zoom, live animation and any viewport size. There are no static
screen coordinates or hand-placed hotspots.

### Front, rear and orientation

`V` animates to the spec's `Front`/`Rear` camera presets along the shortest
arc. The model is rotated, never mirrored: from behind, the player's left
controls appear on the right of the screen and keep their own labels.
`TestRearViewDoesNotMirrorOrSwapSides` guards this.

### Performance

Geometry is generated once per model and cached. Per frame, `Frame` re-poses
only animated/pressed parts and resolves one colour per part (≈12 µs, 0 allocs);
`Pick` runs only on pointer events (≈19 µs). Models stay between 400 and 4000
faces (tested). Material colours are tone-mapped so black plastics remain
readable on dark terminals.

## Interaction

| Input | Action |
| --- | --- |
| hover | highlight + small tooltip |
| click | select; on connected hardware open the component's panel or diagnostic |
| drag / `W A S d` / arrows (stage focused) | orbit |
| wheel / `+` `-` | zoom |
| double-click / `R` | reset camera |
| `V` | front ↔ rear |
| `[` `]` | previous / next component (keyboard access to everything) |
| `G` | Controller Gallery; `←/→` change model, `Esc` back |

## Model switching

A driver's `Descriptor.VisualID` names its visual. When a device is selected
the stage looks the model up (`visual.Lookup`), resets to its front preset and
binds live input through the descriptor's `InputLayout`. Unknown IDs fall back
to the neutral `generic` model. The gallery is a separate stage mode: it never
adds devices, never changes the active device, and never animates input.

## The ten built-in models

| ID | Controller | Backend (Linux) | Rear controls |
| --- | --- | --- | --- |
| `dualsense` | Sony DualSense (CFI-ZCT1) | hidraw protocol driver | — |
| `dualsense_edge` | Sony DualSense Edge (CFI-ZCP1) | hidraw protocol driver; back and Fn buttons reported | Left/Right back button, trigger stops |
| `xbox_series` | Xbox Wireless Controller (model 1914) | evdev (xpad / hid-microsoft) | battery compartment |
| `xbox_elite2` | Xbox Elite Series 2 (model 1797) | evdev (xpad); P1–P4 when the kernel reports them | P1–P4 paddles, hair-trigger locks |
| `dualshock4` | Sony DualShock 4 (CUH-ZCT1/ZCT2) | evdev (hid-playstation / hid-sony) | — |
| `switch_pro` | Nintendo Switch Pro Controller (HAC-013) | evdev (hid-nintendo) | — |
| `8bitdo_ultimate_bt` | 8BitDo Ultimate Bluetooth | evdev; name match, probable | two back paddles (not reported separately), mode switch |
| `gamesir_g7se` | GameSir G7 SE | evdev (xpad); name match | L4, R4 (not reported separately), quick latches |
| `razer_wolverine_v3pro` | Razer Wolverine V3 Pro | evdev (xpad, Linux 7.0 IDs) | four back buttons (not reported separately), HyperTrigger switches |
| `msi_gc20` | MSI Force GC20 V2 | evdev (xpad / hid-generic) | — |

The current feature support per model is in [SUPPORT_MATRIX.md](SUPPORT_MATRIX.md).

Approximate or unverified details are listed in each spec's `Caveats` and
per-component `Note`, and are shown in the gallery. Notably:

- Elite paddle numbering follows Microsoft; positions follow SDL's
  community-confirmed mapping rather than an official diagram.
- Razer's M1–M6 numbering differs between sources, so neutral positional names
  are used and the function button is not modelled.
- 8BitDo's rear-paddle naming, mode-switch position and dimensions are unverified.
- GameSir does not state the mute button, quick latch or port positions.
- Switch Pro player LED placement is unverified.
- MSI dimensions (156 × 105 × 62.5 mm) and mode LEDs are verified; colour trim is stylised.

## Licensing

All geometry is original procedural code (MIT, like the rest of TuiSense).
No manufacturer CAD files or third-party 3D assets are bundled or downloaded.
The models are low-poly interpretations built from public product pages and
manuals, listed in each spec's `References`; they are not official assets.
If you contribute an asset-backed model, it must carry a licence that allows
redistribution, and its attribution must go in `Attribution`.

## Adding a model

1. Create `internal/ui/visual/models_<brand>.go` and call `Register` from `init`.
2. Describe the half silhouette in millimetres, then add components with the
   builders in `parts.go` (or plain `Component` literals). Use the
   manufacturer's labels only where you have verified them; otherwise use a
   neutral description and set `Verify`/`Note`.
3. Give rear buttons and paddles `KindRearButton`/`KindRearPaddle` and the
   `Rear` surface. Set `Bound` only when a TuiSense driver reports that input.
4. If a driver renders with the model, set its `Descriptor.VisualID` and list
   the driver's ModelID in `HardwareModelIDs`.
5. Add the model's labels, notes and caveats to `internal/i18n/tr_models.go`.
6. Preview it at full resolution:

   ```sh
   TUISENSE_MODEL_PREVIEWS=/tmp/models go test ./internal/ui/visual -run Preview
   ```

   This writes front, rear, side and top-rear PNGs for every model.
7. `go test ./...` checks geometry budgets, distinctness from every other
   model, surface placement, picking at every component, framing, translation
   coverage and descriptor ↔ visual consistency.

To list a new model in the gallery set `Gallery: true`; models registered
after the built-in ten are appended in registration order.
