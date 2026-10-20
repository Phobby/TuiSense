# Adding a Controller to TuiSense

TuiSense's UI is generated from a controller's **Descriptor** and **capabilities**.
Adding a controller never requires editing panels, the dashboard, or the keymap.

## 1. Describe the model

Create `internal/drivers/<family>/descriptor.go`:

```go
func Descriptor() device.Descriptor {
    return device.Descriptor{
        ModelID:     "nintendo.switch_pro",
        Family:      "switchpro",          // scopes saved profiles
        Brand:       "Nintendo",
        DisplayName: "Switch Pro Controller",
        ShortName:   "Switch Pro",
        VisualID:    "switchpro",          // see step 3 (or reuse "xbox"/"dualsense")
        Accent:      device.RGB{R: 230, G: 60, B: 60},
        Regions: []device.Region{
            {ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick",
             Capability: device.CapSticks, Slot: device.SlotMidLeft,
             Description: "Deadzones and response curves."},
            // ...
        },
        Capabilities: device.NewCapabilitySet(
            device.CapabilitySpec{ID: device.CapBattery, Support: device.Full},
            device.CapabilitySpec{ID: device.CapHaptics, Support: device.Limited,
                Features: []device.Feature{device.FeatDualMotors},
                Note: "HD rumble is approximated with two motors."},
            device.CapabilitySpec{ID: device.CapLighting, Support: device.Unsupported,
                Note: "Only player LEDs, not configurable yet."},
        ),
        Actions: []device.Action{{ID: "pulse_test", Label: "Test Rumble",
            Capability: device.CapHaptics, Command: device.Command{Type: device.CmdPulseHaptics}}},
    }
}
```

Rules of thumb:

* Declare **every** capability you know about; use `Unsupported` + `Note` so the
  UI can explain *why* something is missing.
* Prefer `Limited` + features over inventing new capabilities.
* A region's `Capability` decides which panel opens when it is clicked.

### Diagnostics contract (`Inputs`)

```go
Inputs: device.InputLayout{
    Buttons:       []device.ButtonDef{{ID: device.ButtonSouth, Label: "B", Group: "Face"} /* … */},
    StickLabels:   [2]string{"Left stick", "Right stick"},
    StickRange:    device.AxisRange{Min: 0, Max: 4095, Center: 2047.5},
    Drift:         device.DriftHeuristics{CenterTolerance: 40, NoiseTolerance: 12,
                       Rationale: "Heuristic for a 12-bit ADC; not a vendor specification."},
    // TriggerLabels/TriggerRange: leave empty — Switch Pro ZL/ZR are digital-only
    // (declare them as ButtonL2Digital/ButtonR2Digital buttons instead).
    BatteryStep:   25,
    Audio:         device.AudioMatch{USBIDs: []string{"057e:2009"}}, // only if it has USB audio
},
```

Fill `Telemetry.Raw`, `Buttons`, and (if possible) `ButtonsLatched`,
`ReportsDropped`, `ReadErrors`. Real backends should also attach every report
since the last poll as `Telemetry.Samples` (an empty, non-nil slice when
nothing new arrived) so diagnostics measure at the device's real rate and never
count stale data. Declare per-axis ranges in `StickAxes` / `TriggerAxes` when
they differ, set `AxisRange.Invert` for axes that report "up" as a high value,
and set `TriggerKind` (`TriggerDigital`, `TriggerSharedAxis`) when the
triggers are not one analog axis each. Implement `device.BatteryDetailsProvider` only
with values the device/OS really reports. The diagnostics UI then shows exactly
the tests your declaration supports.

## 2. Implement the device

Implement `device.Device` (and optionally `device.Simulator` for mocks). Apply
only the `Config` sections your capabilities support; ignore the rest.

## 3. Provide a visual (optional)

Add `internal/ui/visual/<family>.go`, register it in `init()` with
`Register("<visual id>", v)`, build the mesh with `meshBuilder.box/disk`, and
return one `Anchor` per region so 3D picking works.

### Linux evdev gamepads

Controllers that Linux already exposes as `/dev/input/event*` gamepads (xpad,
hid-generic, …) need no protocol driver: add a `gamepad.Model` (see
`internal/drivers/msigc20`) with `Identify` (never claim a match you cannot
support with evidence — return `Probable` for shared IDs), `Describe` (labels,
visual, only real capabilities via `gamepad.Capabilities`) and an optional
`ModeHint`. Inputs are mapped from what the kernel reports, so different input
modes of one controller work without per-mode tables. Pass the model to
`gamepad.NewDriver` in `builtin.go`.

## 4. Register it

Hardware: implement `device.Driver` (`Discover` + `Open`).
Mock/test: return a `device.VirtualDevice`.

Then add one line to `internal/drivers/builtin.go`.

## 5. Run the contract tests

```bash
go test ./internal/drivers/ ./internal/state/ ./internal/ui/...
```

`TestDescriptorsAreConsistent` fails if a region has no panel or no 3D anchor,
or if the descriptor is missing identity fields.
