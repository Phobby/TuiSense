# Windows support, compatibility and test protocol

Status: **experimental**. Everything below is covered by automated tests
(unit, headless UI and, on Windows CI runners, native build, installer and
startup tests). **No controller has been verified on physical Windows
hardware yet.** Nothing in this document claims otherwise.

## Architecture

```
Windows HID listing (SetupAPI + hid.dll, no read/write access)
        │ VID/PID/version, usage, USB strings
        ▼
identity resolution (internal/drivers/winpad) ── model matchers shared with Linux
        │ exact / confirmed → model name, brand, 3D model
        │ probable / none   → "Generic Gamepad", confirmation offered
        ▼
backend
  • XInput slot  → windows-xinput: input, rumble, battery (if reported)
  • HID only     → windows-hid-identity: detected, no input, no commands
        ▼
capabilities (backend's own + model's other hardware as "present, inaccessible")
        ▼
panels, diagnostics, 3D model, Driver & Compatibility
```

Rules:

* Only Windows' built-in drivers and documented APIs are used. TuiSense never
  installs, replaces or configures a driver on Windows.
* A model name is shown only for exact evidence (a documented vendor/product
  ID, MSI's vendor ID 0db0, or USB strings naming the model) or the user's
  confirmation. Button layout, axis count, the XInput API or a generic Windows
  name such as "Controller (XBOX 360 For Windows)" never identify a model or a
  manufacturer.
* An unidentified controller is *Generic Gamepad* with the neutral 3D model.
  No code path selects the DualSense for unknown hardware; the simulated
  DualSense exists only with `--mock` / `--test-devices`.
* XInput does not say which HID entry belongs to which slot. The HID identity
  (entries whose path contains `IG_`) is used only when the number of entries
  equals the number of connected slots and all have the same IDs. Two
  identical controllers stay two devices (one per slot); mixed controllers
  stay generic.
* `IG_` entries are never listed by the HID driver, so TuiSense's own two
  backends do not show one controller twice. Virtual pads created by other
  software (Steam Input, remappers) cannot be linked to the physical controller
  through public APIs; they may appear as an additional generic XInput pad.
* A gallery preview never hides a controller connected later: hot-plugged
  hardware becomes the active device.

## MSI Force GC20 V2

From MSI's manual (GC30 + GC20, `download.msi.com/archive/mnu_exe/gear/GC30%2BGC20_manual.pdf`):

| Mode | LEDs | Manual says | Windows behaviour (expected, not hardware-verified) |
| --- | --- | --- | --- |
| Default | 1 + 2 | "default settings" | SDL's database lists the GC20 V2 as `045e:028e` v`0210` (Xbox 360 protocol) → XInput. Shown as *Generic Gamepad (XInput)*, MSI offered for confirmation; after confirming, MSI name and 3D model |
| Analog | 1 + 3 | "DirectInput Analog Mode" | HID gamepad, invisible to XInput → detected by `windows-hid-identity`, no input |
| Digital | 1 + 4 | "DirectInput Digital Mode" | as Analog |
| Android | 3 + 4 | for Android devices | HID gamepad → detected, no input |

The manual does not name XInput for Default mode, and the IDs MSI uses in the
DirectInput modes are not documented; TuiSense identifies those modes only from
MSI's vendor ID or USB strings naming the GC20. Trigger representation per
mode (separate analog axes in XInput; unknown in DirectInput on Windows) must
be confirmed on hardware. TuiSense never switches modes and never sends MSI
commands.

## Controller compatibility (Windows)

| Controller | Windows path | Identification | Input | Rumble | Battery | Model features | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Sony DualSense | HID | 054c:0ce6 | no | no | no | inaccessible | Partially supported (detection only), automated tests |
| Sony DualSense Edge | HID | 054c:0df2 | no | no | no | inaccessible | Partially supported (detection only), automated tests |
| Xbox Wireless (Series X\|S) | XInput | 045e:0b12 / 0b13 via `IG_` entry | yes | yes | when XInput reports it | — | Implementation available, unverified |
| Xbox Elite Series 2 | XInput | 045e:0b00 / 0b05 / 0b22 | yes | yes (trigger motors no) | when reported | paddles, profiles inaccessible | Implementation available, unverified |
| Sony DualShock 4 | HID | 054c:05c4 / 09cc / 0ba0 | no | no | no | inaccessible | Partially supported (detection only), automated tests |
| Nintendo Switch Pro | HID | 057e:2009 | no | no | no | inaccessible | Partially supported (detection only), automated tests |
| 8BitDo Ultimate Bluetooth | XInput (X mode) | name match only (often unavailable through XInput) → usually generic | yes | yes | when reported | — | Implementation available, unverified |
| GameSir G7 SE | XInput | name match only → usually generic | yes | yes | — | — | Implementation available, unverified |
| Razer Wolverine V3 Pro | XInput | 1532:0a57 / 0a59 | yes | yes | when reported | extra buttons not readable | Implementation available, unverified |
| MSI Force GC20 V2 | XInput (Default) / HID (other modes) | probable 045e:028e v0210; exact by 0db0 or USB strings | Default mode only | Default mode only | — (wired) | — | Implementation available, unverified |

Not on Windows for any model: lighting, adaptive triggers, motion, touchpad,
audio, vendor remapping, on-board profiles, firmware information. They are
declared as present-but-inaccessible where the controller has them and are
explained in Device Details → Driver & Compatibility.

## Diagnostics

```powershell
tuisense doctor --windows            # controllers, IDs, identity, backend, 3D model
tuisense doctor --windows --export   # also saves a report to share
```

The report contains IDs, USB product strings, the chosen backend, registry
entry and 3D model, available and unavailable features, and errors. It does
not contain serial numbers, Bluetooth addresses, device instance paths,
environment variables or your user name/folder. No administrator rights are
needed.

## Automated coverage

| Area | Where | Runs on |
| --- | --- | --- |
| Identity resolution (MSI, Xbox, Elite, unknown, ambiguous, identical pads, confirmation) | `internal/drivers/xinput/identity_test.go`, `internal/drivers/winpad` | all OS |
| No fake DualSense / no Sony branding / no unsupported panels | `internal/app/windows_identity_test.go` | all OS |
| Preview replaced by hot-plugged hardware | `internal/session/manager_test.go` | all OS |
| Real SetupAPI/hid.dll enumeration | `internal/system/winhid` | Windows CI |
| install.ps1: install, reinstall, uninstall keeps data, purge, unpinned signer, missing artifact, corrupt ZIP, path with spaces and non-ASCII | `internal/installtest/install_windows_test.go` | Windows CI |
| Packaged ZIP starts; published release installs on clean runners (x64, ARM64) | `release.yml`, `windows-install.yml` | Windows CI |

## Manual hardware test matrix

Record for every row: date, Windows edition and build (`winver`), TuiSense
version, controller and mode (LEDs), `tuisense doctor --windows --export`
report, expected and actual result, Pass / Fail / Blocked. Do not mark a row
passed without running it.

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| A | Fresh Windows, install.ps1 one-liner | verified install, `tuisense --version` works in a new terminal | Pending |
| B | Portable ZIP from the browser | SHA-256 matches; SmartScreen may warn; runs from Windows Terminal | Pending |
| C | install.ps1 downloaded, inspected, run | as A | Pending |
| D | Start with no controller | "No controller connected", no model, no DualSense | Pending |
| E | Connect MSI GC20 V2 (Default, LEDs 1+2) | *Generic Gamepad (XInput)*, sticks/triggers/buttons live, rumble test, "Is this your MSI Force GC20 V2?" | Pending |
| F | MSI Analog / Digital / Android modes | detected (MSI if 0db0/strings, else Generic Gamepad), "input access is unavailable", no panels | Pending |
| G | Disconnect / reconnect MSI; mode switch while running | removed and re-detected within ~2 s, no stale name or model | Pending |
| H | Connect DualSense (USB, Bluetooth) | "DualSense Wireless Controller", no input, no settings | Pending |
| I | Switch between MSI and DualSense | name, model and panels follow the selection | Pending |
| J | Two controllers (incl. two identical) | both listed, separately selectable | Pending |
| K | Diagnostics on MSI (Default) | sticks, buttons, triggers (0–255 each), rumble | Pending |
| L | EZ / Advanced | no hidden or unsupported features appear | Pending |
| M | Controller-specific settings | none for XInput pads; none for HID-only pads | Pending |
| N | Health report | generated, no absent features | Pending |
| O | Restart TuiSense with controller connected | same identity (confirmation remembered) | Pending |
| P | `tuisense update --check` | only `windows_<arch>.zip` considered | Pending |
| Q | Uninstall (`-Uninstall`) and reinstall | settings and profiles kept | Pending |
