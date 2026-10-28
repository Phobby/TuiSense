# UI audit and minimal redesign

## Audit scope and findings

The audit covered the checked-out implementation, including existing uncommitted
Diagnostics 2.0, firmware and clipboard work. No screenshot was attached to the
request available in this session.

| Surface | Finding | Change |
| --- | --- | --- |
| Startup and device selector | CLI enabled fallback; app registered two test devices automatically | Hardware-only default; separate explicit demo and development flags |
| Discovery | Initial scan could open duplicate keys; DualSense used node identity only | Initial key guard, physical identity filtering and endpoint replacement |
| Header | Count, source pill and device selection repeated below model | One compact selector; source label only for simulated devices |
| Dashboard | Three columns allocated large mostly empty status/info containers | Centered full-width stage and compact status/actions |
| 3D viewer | Every region labeled; title, toolbar and second device selector repeated | Descriptor-driven triangle meshes for ten controllers, face-level picking, front/rear presets, hover tooltip and selected label only (see 3D_MODELS.md) |
| EZ | Lighting, vibration and triggers all visible immediately | Three entry actions; simple presets appear only in the selected category |
| Advanced | More panels and raw input permanently visible | Same calm Home; technical controls are contextual |
| Navigation | Every capability was a top-level destination | Five stable groups; capability-filtered, wrapped secondary controls |
| Diagnostics | Existing tests, report copying, history and scrollable body were valuable | Retained; accessible from primary navigation in both modes |
| Lighting | Swatches/animations could truncate horizontally | Wrap options and add keyboard activation |
| Vibration and triggers | Persistent Home settings and truncated trigger presets | Dedicated contextual controls; wrapped trigger modes/presets |
| Sticks and motion | Raw values and previews distracted from Home | Advanced contextual views; preview-only game-input warning retained |
| Profiles | Existing family-aware save/load functionality | Dedicated destination; simplified preset selection in EZ |
| Device details and firmware | Valuable but rarely needed technical information | Device/Settings group; safe read-only checks retained |
| Settings | Simulation controls exposed to hardware sessions | Show simulator controls only for an active simulated device |
| Status | Repeated status and hardcoded initial hardware battery/charging | One Home status, no assumed battery percentage or USB charging state |
| Logs and footer | Last log plus long legend occupied permanent rows | Logs remain in Advanced Device/Settings; footer is only Help |
| Help and shortcuts | Fixed-height list silently omitted entries; D could enable demo | Scrollable categorized Help; stable primary keys; simulation requires explicit device |
| Notifications | Mode/device navigation generated routine toasts | Removed routine mode/device switch and connection-success notifications |
| Small terminals | Side panels competed with model and clipped controls | Full-width details; translated scroll viewport for long forms |

## Interaction model

- `1`: Home. Model, supported battery telemetry, transport, profile, quick actions.
- `2`: Diagnostics. Existing guided tests, individual reports and final health report.
- `3`: Customize. Supported categories and component choices. Hidden if no editable capabilities.
- `4`: Profiles, where supported.
- `5`: Device / Settings. Preferences plus details, hardware properties and logs as applicable.
- `F` opens device details, `P` profiles, `E` switches UX mode.
- `C` opens the scroll-following selector; arrows/Enter select, Escape closes.
- `?` opens Help. Arrows, Page Up/Down, Home/End and wheel scroll; Escape/? close.
- Escape returns Home and clears selection there. Q quits; typing in profile names takes precedence.

Home has no permanent side cards. At wide sizes a selected customization category
shows a controller stage beside its inspector. Short terminals use the full width
for forms. All underlying device-capability checks remain in the catalog and
backend. A software-only stick preview still explicitly says it does not modify
game input.

## Identity and lifecycle policy

A valid nonzero DualSense unique address is normalized and hashed for its key.
Without it, a resolved sysfs HID/USB ancestor is used when available. Unknown
identity falls back to the individual node: this intentionally avoids merging two
identical controllers. Accessible USB is preferred when two transports report
the same reliable identity. Endpoint changes reopen the backend without adding
another selection. Repeated initial and hotplug discoveries do not duplicate keys.
The generic driver excludes Sony hardware, virtual input by default, and repeated
nodes for the same resolved input directory. It deliberately does not collapse
multiple controllers behind a shared USB adapter by VID/PID or USB parent alone.

Disconnect selects remaining real hardware, or the empty state. It never silently
activates a demo. Per-device configurations remain isolated, and the existing
key-bound dispatcher continues routing commands to the selected backend.

## Validation and limits

Automated tests use injected hardware drivers and synthetic sysfs; they do not
claim physical controller validation. Tests cover startup policy, explicit demo,
duplicate identity, unreliable identity, endpoint changes, hotplug, selection,
contextual views, mode changes, Help capture and scrolling, footer removal,
keyboard controls, and renderer bounds. The full existing diagnostics, firmware,
profiles and clipboard suite remains part of validation.

Headless Limoni rendering covers 80x24, 100x30, 120x40, 160x45 and 220x60, in both
modes across every available panel. Set `TUISENSE_UI_ARTIFACTS` to a directory when
running the responsive regression test to export text snapshots for inspection.
These snapshots omit color; PTY smoke tests additionally check executable startup
and exit. Hardware rumble, USB/Bluetooth transitions and firmware queries still
need validation with a physically connected device.

When identity metadata is absent or unreliable, transport-level deduplication
cannot safely be guaranteed. Distinct ambiguous nodes remain separate instead of
silently hiding a real controller.

## Verification record — 2026-10-26

- `go test ./...`: passed, including all existing diagnostics and firmware suites.
- `make build`: passed.
- `git diff --check`: passed.
- Responsive headless UI: both modes, every available panel at all five sizes above;
  checked accessibility bounds, model visibility and the minimal footer.
- PTY smoke sessions: normal and explicit demo startup at all five sizes; exercised
  Help, scrolling, navigation and Q; every process exited with status 0, no panic.
- `tuisense debug devices`: zero DualSense devices and zero evdev gamepads in this
  environment. No physical hardware behavior is claimed as tested.
- Installed executable updated at `~/.local/bin/tuisense`;
  SHA-256 matched the repository build and `tuisense --version` succeeded.
