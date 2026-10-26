# Diagnostics 2.0

Diagnostics reuse the universal device registry, `InputLayout`, runtime capabilities,
and the existing Limoni controller stage. No test calibrates hardware or changes
input delivered to games. Xbox remains a simulated development driver.

## Using diagnostics

Open Diagnostics, choose a category or run the guided check. The guided order is
connection, buttons, sticks, triggers, vibration, and other available capabilities.
Measurements and active effects start only on user action. `o` starts the current
test, `x` cancels, `h`/`l` choose categories, `g` starts the guide, and `n` advances.
Changing category stops active effects. Skipped buttons and unfinished movement
are incomplete observations, never proof of a defect.

The controller stage stays outside the scrolling inspector. Use the mouse wheel,
PgUp/PgDn, or the inspector's up/down buttons to reach details on small screens.
Copyable rows and controls participate in keyboard focus. Enter/Space copies the
focused row; `y` copies the current test, `Y` copies all diagnostics.

Stick tools include released-center measurements, guided directional travel,
return to center, a separate circularity view, and the existing snapback analyzer.
The deadzone playground has a 0–25% slider, presets, one-percent controls, and
radial/axial filtering. Cyan is raw input, green is filtered input, pink is the
measured initial resting center. The yellow boundary is the selected preview.
Filtering is a simulation only; persistent stick settings are separate.

## Measurement and interpretation

- Bipolar normalization scales each side of the declared center separately and
  honors axis inversion. Trigger axes use their complete declared unipolar range.
  Evdev centers are nominal range midpoints, not measured hardware calibration.
- Real backends supply timestamped reports. Evdev is event-driven and may repeat
  the last known state with `Held` while quiet. Such observations contribute to
  resting-position statistics but not to measured fresh-report frequency. Kernel
  fuzz can suppress small movements; absent noise does not prove a noiseless sensor.
- Means and population standard deviations use Welford accumulation. Center
  deviation is the Euclidean distance of the mean normalized position. Jitter is
  the larger axis standard deviation. Stability compares first/second half means.
  The median uses the most recent 4,096 resting deviations; total count, mean,
  extrema, and variance use all observed samples. Live trails and events are bounded.
- Resting interpretations use driver-declared heuristics. Existing generic evdev
  expectations are 3% center and 1% noise; these are **not manufacturer limits**.
  The driver's rationale is included in reports. Offset must repeat in two resting
  measurements before the analyzer suggests possible drift. It never declares a
  hardware failure from stick measurements. Preview deadzone size does not change
  the measured values or hardware interpretation.
- Directional travel completion uses 85% of declared travel. Circular coverage
  counts 16 sectors reaching 70% radius; 14 sectors completes a rotation. These
  values guide test execution, not hardware quality. Circularity reports the
  observed radius distribution and directional asymmetry; square gates are valid.
- Trigger completion uses 95% of declared analog range (90% of half a shared axis).
  Rest interpretation uses 5%, noise 2%, return difference 3% or four resting
  standard deviations, and report jumps 20%. These are execution/interpretation
  heuristics, not calibrated controller specifications. Incomplete presses and
  unobserved slow movement are incomplete tests. Digital mappings skip analog
  smoothness/range checks. Shared-axis direction is learned during the left press;
  simultaneous triggers cannot be measured independently.
- Missing mapping ranges are unavailable. No data is not a centered measurement.
  Measured sample frequency is not USB polling rate or end-to-end latency. Hardware
  timing, battery wear/cycles, and inaccessible audio endpoints remain unavailable.

## MSI input mode assistance

Observed kernel capabilities determine axis/button mappings. Mode names are
explicitly unverified inferences; check the physical LEDs. Generic joystick
buttons keep neutral labels when physical positions are unknown. Retest Input
Mapping schedules a close/reopen on the device worker, rereads capabilities,
and clears incompatible measurements. Hotplug re-enumeration is also supported.

Instructions are verified against MSI's official sources:

- https://www.msi.com/faq/6413
- https://download.msi.com/archive/mnu_exe/gear/GC30%2BGC20_manual.pdf

Default LEDs 1+2; hold Mode 3 seconds for Analog (1+3); from Analog press Mode
1 second for Digital (1+4); from Digital hold Mode 3 seconds for Android (3+4).
TuiSense does not remotely switch modes or send undocumented HID commands.

## Active test safety

Vibration uses bounded compatibility rumble pulses (at most 350 ms per pulse,
maximum level 230/255). Independent motor controls appear only when declared by
the backend. Cancellation silences diagnostic motors, including evdev kernel
force-feedback effects. Disconnect closes the device; category/panel changes and
shutdown clean up active tests. Late UI ticks do not replay expired patterns.
The saved user profile is not modified. Advanced DualSense audio haptics are not
measured by the compatibility rumble diagnostic.

## Sharing and local history

Export writes private `.json`, `.md`, and `.txt` files off the UI thread, by default under
`$XDG_DATA_HOME/tuisense/reports`. Reports include algorithm version, mapping
fingerprint, actual measurements, status, test timestamps, limitations, and recent
events. Unique identifiers are redacted by default, including those embedded in
explanations. Microphone recordings are excluded. Clipboard uses existing native
platform adapters (`wl-copy`, `xclip`, `xsel`, `pbcopy`, PowerShell). Optional OSC 52
requests are explicitly unacknowledged; a request is not reported as confirmed copy.

History is **manual opt-in per snapshot**: Save snapshot, Compare history, Clear
history. No polling code collects history or uploads it. At most 256 snapshots are
kept under `$XDG_DATA_HOME/tuisense/diagnostics`. A custom config root also isolates
history and reports (useful for portable installations and tests). Files use owner
permissions. Device identity is HMAC-SHA256 with a random installation-local salt;
raw identifiers are not stored. Without a stable identifier, comparisons are limited
to the current device path and identical controllers may be indistinguishable.

Comparison checks identity, simulation, connection, input mode, mapping fingerprint,
algorithm, and method (including resting duration, preliminary/full test, circularity,
event-driven source, and language). Incompatible snapshots produce an explanation
instead of a trend. Compatible completed measurements show previous, current, and
signed difference. A pair of observations never proves deterioration. Clear history
removes the snapshots; the local salt is retained for future identity consistency.

## Validation boundary

Deterministic simulated streams and headless Limoni tests exercise calculations,
mode/capability selection, input transitions, cleanup, copy failures, exports, and
history. The optional `/dev/uinput` integration test requires explicit opt-in and
is not a physical controller test. Real DualSense/MSI behavior, physical LED mode
confirmation, desktop clipboard permissions, acoustic output, motor perception,
and USB/Bluetooth sampling characteristics require hardware/manual validation.

## Explainable final reports

Schema 2.1 and algorithm diagnostics-2.1.0 extend reports with separate protocol
completion and per-component evidence. See [Health reports](HEALTH_REPORTS.md) for
scoped copy actions, automatic guided summaries, historical report review, and
TXT/Markdown/JSON export behavior.
