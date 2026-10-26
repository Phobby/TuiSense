# Explainable health reports

The Health Report tab combines existing diagnostic observations. The guided
workflow opens it automatically after its last step. Generate Health Report also
creates a partial snapshot at any point. Completing navigation never makes an
unperformed test pass. Report generation never actuates or calibrates hardware.
The original procedural DualSense model and the DualSense/Xbox test devices remain.

## Architecture

- `diag.Result` retains measurements and original engine status.
- `diag.Completion` tracks protocol coverage separately from normal/abnormal input.
- `diag.Assessment` records component, observation, evidence, comparison basis,
  reason, relevant possible explanations, gameplay impact, actions, and limitations.
- `diag.BuildReport` assembles these into the existing `diag.Report`; schema 2.2
  and algorithm `diagnostics-2.2.0` identify the assessment semantics.
- `report_simple.go` writes the default Turkish health report, compact category
  reports and warnings-only copy. It selects important measurements without
  introducing fault thresholds. Stick and trigger observations retain component
  labels supplied by the driver; snapback evidence is captured in the snapshot.
- `report_format.go` provides explicit technical text/Markdown access. JSON
  retains original assessments, criteria, raw measurements and mapping metadata.
  Exported event lists contain at most 32 distinct category/message pairs, keeping
  the latest occurrence. The original session/log buffers are not modified.
- `health_report.go` renders the simple report by default in both EZ and Advanced
  modes. Technical Report is a separate explicit toggle. Raw Logs opens the
  existing log inspector. Reports scroll independently of the controller stage.
- `HistoryStore` stores optional report snapshots; `diagnostic_storage.go` performs
  history reads/writes and exports in its existing background worker. Building a
  report is bounded, synchronous, read-only work over the current measurement
  snapshot; it performs no filesystem, network, audio, or hardware I/O.

The builder remains capability driven. It contains no model-name dispatch.
Actual device ranges, trigger kind, labels, and optional physical mode guidance
come from the driver descriptor. Firmware and deadzone preview have their own
scoped reports; neither is interpreted as proof of healthy hardware.

## Coverage and assessment

Statuses and completion answer different questions. A completed protocol can
produce an inconclusive observation or warning. A normal initial stick rest is
only a preliminary observation until movement and return phases are complete.
Cancelled tests retain their incomplete status and available partial observations.
No hardware health score is generated.

The stick assessment reuses the existing typed verdict and measured repeatability.
Two resting measurements can justify Possible Issue, never a confirmed defect.
The driver's heuristic basis is included explicitly, without changing thresholds
for reporting. Incomplete movement is not a failed stick. Circularity or snapback
alone does not complete the entire resting/movement/return protocol.

The trigger assessment distinguishes digital, separate analog, and shared-axis
mappings. Incomplete presses, missing baselines, or unfinished slow movement are
incomplete evidence. Digital mappings do not inherit analog thresholds. Physical
mode recommendations appear only when a driver supplies mode documentation.
An audio I/O error is an operational failure; without measured hardware evidence
it does not establish a physical microphone or speaker defect.

A connected entry without observed reports cannot verify a report-based input
stream. Quiet event-driven input is handled separately. Held evdev states are not
independent hardware samples; kernel filtering may hide small sensor noise.

Unsupported MSI features such as battery, motion, microphone, touchpad, and
firmware metadata remain unsupported, not faulty. Battery level does not imply
battery health. The builder does not invent wear, cycles, capacity, or charging
power. Simulated results always retain their source marker.

## UI and copying

Every available diagnostic category offers a compact Test Report and an independent
copy action. The final report offers simple report, technical report, and warnings
copy actions. The report language is Turkish regardless of UI language. The simple
report contains Device metadata, Overall Result, meaningful Potential Issues only,
a three-column Results table, prioritized Actions, and a short Final Assessment.
Firmware Information also exposes scoped copying. TXT copies have no Markdown
heading markers or terminal decorations; Markdown exports preserve headings and
tables. Default exports produce simple TXT/Markdown and structured technical JSON.
Unknown measurements are omitted from the short report, not invented. Technical
measurement names and original JSON fields remain available for troubleshooting.

All actions copy underlying content, including offscreen details, through the
existing ClipboardService. Enter/Space activates focused report actions; `y` and
`Y` copy the current/full diagnostic report. Only native acknowledged copies
produce confirmed success; clipboard failures and unacknowledged OSC 52 requests
retain the existing honest feedback behavior. Diagnostic health reports always
redact unique identifiers; identifier opt-in on other device-detail copy surfaces
remains separate.

Absent controller capabilities are omitted from the combined short report; an
explicitly scoped unsupported test remains clearly unavailable, never faulty.
Unavailable capabilities that exist but cannot currently be accessed remain in
the results table. Warning and incomplete protocol coverage are shown together.

## Export and privacy

Export creates private `.txt`, `.md`, and `.json` files for the displayed snapshot.
Text is the default copy format. JSON includes schema/algorithm versions, device
metadata, timestamps, declared input mapping, completion, per-component findings,
evidence, actions, coverage, and missing or unsupported fields. Nonfinite numeric
values are excluded instead of emitted as fabricated measurements.

Reports exclude microphone recordings and unrelated logs. MAC addresses, device
identifiers, sysfs identifiers, and embedded serial/MAC messages are redacted by
default, including assessment strings. Export filenames are unique across repeated
exports of the same historical snapshot. Files remain owner-readable/writable.

## Optional history

Save snapshot is explicit opt-in per save; no automatic upload or collection is
introduced. Review Previous Report, Older report, and Newer report browse local
snapshots for the installation-specific hashed device identity. Copy Previous
Report and Export operate on the stored report, not a reconstruction using current
measurements. Generate Current Report returns to fresh observations.

Full reports retain their original algorithm, schema, coverage, evidence, and
limitations. Legacy snapshots without completion metadata are displayed as limited
historical observations and are not retroactively certified by the new rules.
Comparisons retain the mapping, algorithm, method, connection, mode, and simulation
compatibility checks. Incomplete protocols are excluded from numeric comparisons;
a pair of observations never establishes deterioration. Clear history removes
snapshots and dismisses the historical report. Storage is bounded by 256 entries
and an 8 MiB file limit, retaining fewer old entries when full reports are large.

## Validation limits

Deterministic tests cover complete/partial reports, unsupported features, missing
measurements, normal and repeated-offset sticks, incomplete and digital triggers,
cancellation, simultaneous warnings, identifier redaction, Markdown/JSON/text,
writer/export failures, legacy/full history, mode consistency, keyboard/copy actions,
and compact-screen controller visibility. Existing clipboard backend failure tests
continue to apply. Real DualSense/MSI input modes, sensor repeatability, audio,
haptics, and desktop clipboard permissions still require physical/manual testing.
