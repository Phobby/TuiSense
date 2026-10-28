# Controller support, drivers and capabilities

This document explains how TuiSense decides what each connected controller
can do, which operating-system drivers it relies on, and how driver setup is
handled safely. The per-model summary lives in
[SUPPORT_MATRIX.md](SUPPORT_MATRIX.md), generated from the same descriptors.

## Capability engine

Every capability of a controller is a `device.CapabilitySpec`
(`internal/device/capability.go`) that records separate facts:

| Field | Meaning |
| --- | --- |
| `Presence` | `Present` or `Absent`: does the hardware physically have it |
| `Access` | what TuiSense can do: `AccessRead`, `AccessTest`, `AccessConfigDevice`, `AccessConfigSoftware`, `AccessPreview` |
| `Requires` | why present hardware is out of reach: `driver`, `permission`, `connection`, `backend`, `platform` |
| `Transports` | connections the access applies to (e.g. DualSense audio: USB only) |
| `Backend` | the implementation (`dualsense-hidraw`, `linux-evdev-ff`, `linux-power-supply`, …) |
| `Verification` | `hardware`, `automated`, `experimental` or `unverified` |
| `Persistent` | a device setting survives a disconnect |

`State()` summarises a spec as `HardwareAbsent`, `Detected`, `RequiresDriver`,
`ReadOnly`, `Testable`, `PreviewOnly`, `ConfigurableOnDevice` or
`ConfigurableThroughSoftware`. Physical presence never implies access:
`Supports()` is true only when the hardware is present **and** TuiSense
implements at least one operation.

`CapabilitySet.ForConnection` re-derives the set for the current transport, so
a controller that moves from USB to Bluetooth loses (and later regains) exactly
the features its new connection cannot carry. Backends call it whenever the
transport changes; the UI, diagnostics and the command gate read the result.

TuiSense's capability IDs map to the feature list as follows:
battery/charging → `CapBattery`; sticks, deadzones, curves → `CapSticks`
(features `deadzones`, `response_curve`); rumble / haptics → `CapHaptics`;
adaptive triggers → `CapAdaptiveTriggers`; RGB, player LEDs, mic LED →
`CapLighting` features; gyroscope/accelerometer → `CapMotion` features;
microphone/speaker/headset → `CapAudio` features; touchpad → `CapTouchpad`;
rear buttons and paddles → `CapRearControls`; button remapping and macros →
`CapRemapping`; profiles → `CapProfiles`; firmware information / update check
→ `CapFirmwareInfo` / `CapFirmwareCheck`; calibration → `CapCalibration`;
connection diagnostics → `CapConnection` + `CapDiagnostics`.

## What the UI shows

* Settings panels (`internal/state/panels.go`) require the capability **and**
  a configuring access (`AccessConfigDevice`, or `Preview`/`Software` for the
  stick and motion previews). Hardware TuiSense can only read never gets a
  settings panel.
* The panel title states the scope of its controls: *applied to the
  controller*, *software input processing* or *preview only — games are not
  affected*.
* Diagnostics (`internal/diag/plan.go`) mark absent hardware `Unsupported`
  and present-but-unreachable hardware `Inaccessible`; both are hidden from
  every diagnostics view and omitted from every report (text, Markdown, JSON
  and the Turkish health report). A missing battery is never a failure.
* Present-but-unreachable features are explained only in
  **Device Details → Driver & Compatibility**, with a remedy when one exists.
* 3D components open the panel or test of their capability region only when
  the connected device declares that region; components whose input the
  driver does not report are labelled *visual only*.

## Command gate

`device.Operations` (`internal/device/operations.go`) declares every command:
required capability, feature and access, scope (device / software / preview /
test), persistence and revertibility. `device.Authorize` is enforced by the
command dispatcher with the device's capabilities read at execution time, so
an unsupported command never reaches a backend even if a view were to send
it. `internal/session/authorize_test.go` proves it.

## Backends

| Backend | Platforms | Controllers | Package |
| --- | --- | --- | --- |
| DualSense hidraw protocol | Linux | DualSense, DualSense Edge | `internal/drivers/dualsense` |
| evdev gamepad (+ kernel force feedback, kernel power_supply) | Linux | Xbox Wireless, Xbox Elite Series 2, DualShock 4, Switch Pro, 8BitDo Ultimate Bluetooth, GameSir G7 SE, Razer Wolverine V3 Pro, MSI Force GC20 V2, unidentified pads | `internal/drivers/gamepad`, models in `internal/drivers/padmodels` and `internal/drivers/msigc20` |
| Simulated mock / test devices | all | DualSense mock, Xbox test device | `internal/drivers/dualsense`, `internal/drivers/xboxmock` |

The evdev backend reads input, plays bounded rumble pulses through Linux force
feedback when the kernel exposes it, and reads battery level from the kernel's
`power_supply` class. It never sends vendor-specific output reports.

Identification uses documented vendor:product IDs (with their source in the
model spec) where known; otherwise device-name evidence, marked *probable*
when it is not unique (the user can confirm it in Diagnostics → Connection).
Face buttons follow the kernel's convention per driver: `hid-playstation`,
`hid-sony` and `hid-nintendo` report positions (BTN_NORTH is the top button),
`xpad` keeps the historical Xbox layout.

### Windows and macOS

TuiSense builds and runs on Windows and macOS (gallery, profiles, reports,
Driver & Compatibility explains the situation), but no controller backend is
implemented there. Candidate APIs for future backends are HID / XInput /
Windows.Gaming.Input / GameInput on Windows and IOKit HID / GameController on
macOS. A backend must declare its capabilities with `Requires: platform`
elsewhere; nothing is claimed until it exists and is tested.

## Driver & Compatibility and safe setup

`internal/compat` analyses the connected controller read-only (loaded kernel
driver, expected drivers, input/output permission, force feedback, battery,
audio, rear inputs, uinput) and lists only actionable findings.

Remedies:

| Action | Kind | What it does |
| --- | --- | --- |
| Install the DualSense udev rule | executable after approval | `pkexec install -m 0644 <staged rule> /etc/udev/rules.d/70-tuisense-dualsense.rules`, `pkexec udevadm control --reload-rules` |
| DualSense udev rule (manual) | instructions | the same, when polkit is not available |
| Grant controller access (`input` group) | instructions | `sudo usermod -aG input $USER` |
| xpadneo for Xbox over Bluetooth | instructions | only when rumble is missing over Bluetooth; links the official project |
| Elite Series 2 paddles | instructions | kernel ≥ 6.1 over USB; HID-BPF fix / udev-hid-bpf over Bluetooth |
| Kernel support (Razer Wolverine V3 Pro) | instructions | Linux 7.0 adds its xpad IDs |

Rules enforced in code (`compat.Execute`) and tested in
`internal/compat/compat_test.go`:

1. Nothing runs at startup or without the explicit *Approve and run* step,
   which is only offered for the action under review.
2. Only allowlisted actions can execute; every argument vector is validated,
   and the staged udev rule must be byte-identical to `tuisense --print-udev`.
3. No shell is used; the privileged step goes through polkit's `pkexec`, so the
   TUI never needs administrator rights to start.
4. Kernel modules, DKMS drivers and module blacklists are never installed or
   changed by TuiSense; working drivers are never replaced.
5. Every action lists its system changes and rollback, and the result is
   verified afterwards.

## Verification status

All support listed in the matrix is covered by automated tests (protocol,
mapping, identification, capability and UI tests with synthetic devices). No
controller was verified on physical hardware as part of this work; the matrix
says so explicitly and never claims hardware verification from tests.

## Adding a controller

See [ADDING_A_CONTROLLER.md](ADDING_A_CONTROLLER.md). For a controller that
Linux already serves as an evdev gamepad, a single `gamepad.ModelSpec`
(identification, labels, physical `Hardware`, visual ID) is enough: the
backend derives connection-aware capabilities, diagnostics and the support
matrix row from it.
