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
        VisualID:    "switch_pro",         // a visual.ModelSpec ID, see step 3
        Accent:      device.RGB{R: 230, G: 60, B: 60},
        Regions: []device.Region{
            {ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick",
             Capability: device.CapSticks,
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

## 3. Provide a visual

Visuals are data-only `visual.ModelSpec` descriptors turned into cached 3D
meshes; see [3D_MODELS.md](3D_MODELS.md). Reuse an existing spec when your
controller is one of the ten built-in models (and add your driver's ModelID to
its `HardwareModelIDs`), or register a new spec. Every region your descriptor
declares needs a component with that `Region`, and every button it declares
needs a component bound to that `ButtonID`, so picking and live input work.

### Linux evdev gamepads (sample integration)

Controllers Linux already exposes as `/dev/input/event*` gamepads need no
protocol driver. Describe the model as data in `internal/drivers/padmodels`
(or your own package) and register it in `internal/drivers/builtin.go`:

```go
var MyPad = gamepad.ModelSpec{
    ID: "acme.pro_pad", Family: "acme_pro_pad", Brand: "Acme",
    DisplayName: "Acme Pro Pad", ShortName: "Pro Pad", VisualID: "acme_pro_pad",
    // Documented IDs only, with their source. Without verified IDs use
    // Strings (and Probable: true when the name is not unique).
    IDs: []gamepad.DeviceID{{Vendor: 0x1234, Product: 0x5678, Transport: "usb", Source: "Linux xpad device table"}},
    Labels:        map[device.ButtonID]string{device.ButtonSouth: "A" /* … */},
    TriggerLabels: [2]string{"LT", "RT"},
    // What the product physically has. Absent hardware is never shown;
    // present hardware TuiSense cannot reach is explained in Driver & Compatibility.
    Hardware: gamepad.Hardware{Battery: true, Rumble: true, RearControls: true,
        // Only if the kernel reports rear controls individually:
        // RearInputs: []gamepad.ButtonMap{{Code: 0x2c4, ID: device.ButtonPaddle1, Label: "P1", Group: "Rear"}},
    },
    KernelDrivers: []string{"xpad"},
}
```

The backend then derives, per connection, input mapping, rumble (when the
kernel exposes force feedback), battery (kernel `power_supply`), diagnostics,
the Driver & Compatibility checks and the support-matrix row. Add tests like
`internal/drivers/padmodels/padmodels_test.go` (exact IDs, look-alike
rejection, capabilities) and regenerate the matrix:

```sh
go run ./cmd/tuisense debug support-matrix > docs/SUPPORT_MATRIX.md
```

Never add vendor output reports or configuration commands without a verified
protocol; declare such features as present but `RequiresBackend` instead.
See [DRIVERS.md](DRIVERS.md) for the capability model and safety rules.

## 4. Register it

Hardware: implement `device.Driver` (`Discover` + `Open`).
Mock/test: return a `device.VirtualDevice`.

Then add one line to `internal/drivers/builtin.go`.

## 5. Run the contract tests

```bash
go test ./internal/drivers/ ./internal/state/ ./internal/ui/...
```

Every supported `CapabilitySpec` must declare its `Access` (read, test,
configure on device, configure through software, or preview); a capability
without access is hidden. `TestDescriptorsAreConsistent` fails if a region has no panel or no selectable
3D component, if a declared button has no bound component, or if the
descriptor is missing identity fields. `TestVisualHardwareLinks` checks that a
visual only claims hardware support for drivers that render with it.
