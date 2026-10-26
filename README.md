# TuiSense

**TuiSense** is a terminal-native, **capability-driven controller management platform** built in Go on the [Limoni](https://github.com/thebanri/limoni) TUI & 3D engine. Its centerpiece is an interactive 3D controller in the middle of the screen; everything around it — status, device details, settings panels, even which tabs exist — adapts to the selected controller's declared capabilities and to the chosen **EZ** or **Advanced** mode.

Real backends: the **Sony DualSense** (`054c:0ce6`) and **DualSense Edge** (`054c:0df2`) over Linux hidraw (USB & Bluetooth), and **generic Linux evdev gamepads** with model identification for the **MSI Force GC20 V2** (wired). A simulated **Xbox test controller** ships alongside to demonstrate that the same UI drives very different hardware — it is never confused with real devices.

---

## Highlights

- **Universal device model** — controllers are described by a `Descriptor` (identity, regions, 3D visual, actions) and a `CapabilitySet` (Battery, Connection, Firmware Info, Haptics, Adaptive Triggers, Lighting, Sticks, Motion, Audio, Profiles, Diagnostics, Firmware Update, Firmware Update Check, Remapping, Calibration — each Full / Limited / Unsupported with sub-features).
- **Controller registry** — hardware drivers, mock devices, and test devices are registered in one place (`internal/drivers/builtin.go`). See [docs/ADDING_A_CONTROLLER.md](docs/ADDING_A_CONTROLLER.md).
- **Centered 3D stage** — the controller is the focal point; click its regions (or the callout pills) to open the matching settings. Switch controllers with the chips under the model, the header pill, `C`, or `,` / `.`.
- **EZ Mode** — a calm *Quick Settings* home with a few plain-language choices (light color, vibration strength, trigger feel, profile).
- **Advanced Mode** — full adaptive-trigger editor with force curve, per-motor haptics, RGB picker, stick deadzones & drift scope, IMU, audio, capability matrix, firmware details, logs.
- **Per-device memory** — each controller keeps its own config; the selected mode, last device, and active profile per family persist in `settings.json`.
- **Diagnostics & Device Health Center** — a *Diagnostics* tab with a health summary, guided check, and per-component tests: a 4-step **guided stick test** (release → what the resting position means → full movement → plain-language result: *Normal*, *Possible drift*, *Further testing recommended*, *Inconclusive* — never "broken"), a **snapback test** (how far a stick swings past center after a sudden release, and how fast it settles), an interactive **deadzone preview** (0–25%, raw input vs. effective game input, clearly *preview only*), a guided **trigger test** (rest → full press → smooth movement → return; adapts to analog, digital, or shared-axis triggers; an incomplete press is "Test incomplete", not a fault), buttons, touchpad, motion sensors, vibration, controller microphone & speaker (ALSA, matched by USB ID — never your PC's devices), battery (no fabricated health data), and connection. Measurements use every input report (≈250 Hz on a wired DualSense) and the device's own axis ranges. Advanced mode adds raw values, statistics, circularity and time-series graphs, and JSON/Markdown report export.
- **MSI Force GC20 V2** — real evdev input (sticks, D-pad, A/B/X/Y, LB/RB, LT/RT, stick clicks, Back/Start/Guide), input-mode-aware mapping, Linux rumble when the kernel exposes it, USB details, an MSI visual, and only the capabilities it really has. `tuisense debug devices` / `tuisense debug inputs --watch 15` report identification and input metadata (no serial numbers).
- **Device details & firmware update checker (read-only)** — the *Device Details* tab (or `F`) shows what the controller reports about itself, read automatically from its HID Firmware Info report and Linux sysfs (no terminal commands needed): firmware version, firmware build, build date, raw hardware version, kernel driver, and in Advanced mode every raw field with its origin. It compares the installed version with Sony's published metadata and says *Update available*, *Up to date*, or honestly *Unable to verify* when the controller's firmware target cannot be identified (it is shown as *Unknown*, never guessed). It never downloads or installs firmware; it explains Sony's official methods (PlayStation Accessories on Windows, PS5) and opens or copies Sony's instructions page. See [docs/ARCHITECTURE.md §11b](docs/ARCHITECTURE.md).
- **English & Turkish UI** — switch any time in *Settings → Language* (`English | Türkçe`); the choice is saved, and the first launch follows your system locale.
- **DualSense backend** — native USB (`0x02`) and Bluetooth (`0x31` + CRC32) output reports, input parsing, sysfs battery, udev diagnostics, and a physics-driven mock (`--mock`).

---

## Architecture & Project Structure

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full design: layers, capability model, registry, selection policy, layouts, EZ/Advanced behavior, persistence, and roadmap.

```text
TuiSense/
├── cmd/tuisense/main.go          # CLI flags & bootstrap
├── internal/
│   ├── device/                   # Universal model: capabilities, descriptor, Device/Driver, registry, config
│   ├── drivers/
│   │   ├── builtin.go            # Registers built-in drivers & virtual devices
│   │   ├── dualsense/            # DualSense descriptor, hidraw driver, HID protocol, mock
│   │   ├── gamepad/              # Generic Linux evdev gamepad driver (mapping inference, rumble)
│   │   ├── msigc20/              # MSI Force GC20 V2 identification, descriptor, mode hints
│   │   └── xboxmock/             # Xbox test device (UI/architecture only, no hardware)
│   ├── diag/                     # Diagnostics engine: plan, tests, guided run, reports
│   ├── audio/                    # Controller audio endpoints: ALSA backend + simulated
│   ├── session/                  # Multi-device manager (select, poll, hot-plug) + dispatcher
│   ├── state/                    # AppState, panel catalog & modes, preferences, profiles
│   ├── ui/
│   │   ├── layout.go             # Wide / Stacked / Compact layouts (stage centered)
│   │   ├── visual/               # 3D visual registry: dualsense, xbox, generic
│   │   ├── widgets/              # Stage, status card, device card, help bar
│   │   └── views/                # Dashboard, panel registry, Advanced & EZ panels
│   ├── app/                      # Wiring, event queue, keymap, headless UI tests
│   ├── system/                   # hidraw/sysfs discovery, udev, XDG; evdev/ (input nodes, FF)
│   ├── debugcmd/                 # `tuisense debug devices|inputs`
│   ├── theme/                    # Palettes & card styles
│   └── logging/                  # Ring-buffer logger
├── contrib/70-tuisense-dualsense.rules
├── docs/{ARCHITECTURE.md, ADDING_A_CONTROLLER.md}
├── scripts/install.sh
└── Makefile
```

---

## Requirements & Installation

### Prerequisites
- **OS**: Linux (kernel 5.12+ recommended with `hid-playstation` and `hidraw` support)
- **Go**: Go 1.25+
- **UI Engine**: [`github.com/thebanri/limoni`](https://github.com/thebanri/limoni) `v0.9.4`

### System-Wide User Installation (`~/.local/bin`)

Compile and install `tuisense` into `~/.local/bin` (rootless) so it can be launched from any directory in any terminal:

```bash
git clone https://github.com/Phobby/TuiSense.git
cd TuiSense
make install
```

`make install` compiles a standalone binary, installs it atomically to `~/.local/bin/tuisense`, initializes XDG directories (`~/.config/tuisense/profiles`, `~/.local/share/tuisense`, `~/.cache/tuisense`), and ensures `~/.local/bin` is configured in your shell (`bash`, `zsh`, `fish`, or `sh`) without adding duplicate `PATH` entries.

### Makefile Targets

```bash
make build      # Compile standalone executable to ./bin/tuisense
make install    # Build and install tuisense to ~/.local/bin/tuisense
make uninstall  # Remove ~/.local/bin/tuisense (preserves user profiles)
make test       # Run the complete unit and headless UI test suite
make run        # Build and launch TuiSense
make mock       # Build and launch TuiSense in interactive mock mode
```

---

## Linux Permissions & `udev` Setup

To read input telemetry and write adaptive trigger, haptic, and lightbar output reports to `/dev/hidraw*` without running as `root`, install the included `udev` rules:

```bash
# View the rules directly from the installed command:
tuisense --print-udev

# Install to /etc/udev/rules.d/ and reload:
sudo cp contrib/70-tuisense-dualsense.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Reconnect your DualSense controller via USB-C or Bluetooth after reloading `udev`. If permissions are missing, TuiSense will gracefully display an interactive diagnostic modal (`u`) rather than crashing.

---

## Usage

Once installed, open any terminal from any directory and run:

```bash
# Start interactive dashboard (detects physically connected controllers; no automatic demo devices):
tuisense

# Launch directly with the interactive mock DualSense controller:
tuisense --mock

# Compatibility option (real hardware is already the default):
tuisense --no-mock

# Explicitly expose development test devices:
tuisense --test-devices

# Display available commands, flags, and XDG directories:
tuisense --help

# Display the installed application version:
tuisense --version

# Identify connected controllers (USB IDs, kernel driver, model match):
tuisense debug devices

# Show input capabilities and axis ranges; print live events for 15 s:
tuisense debug inputs --watch 15

# Print the required Linux udev rules:
tuisense --print-udev
```

---

## Controls & Navigation

### Mouse
- **Click a controller region** — selects it and opens the settings for its capability (in EZ mode, Advanced-only areas show a hint instead).
- **Click the header selector** — switch controllers when multiple devices are attached.
- **Click EZ / ADVANCED in the header** — switch mode.
- **Drag / scroll on the stage** — orbit / zoom the 3D model.

### Keyboard
| Key | Action |
| :--- | :--- |
| `1`–`5` | Home, Diagnostics, Customize, Profiles, Device / Settings (unsupported destinations are hidden) |
| `C` | Open the controller selector (`↑/↓`, `Enter`, `Esc`) |
| `,` / `.` | Previous / next controller |
| `E` | Toggle EZ ↔ Advanced mode |
| `P` | Profiles panel |
| `[` / `]` | Cycle controller regions |
| `Tab` / `Shift+Tab` | Move focus between controls |
| `←` / `→`, `↑` / `↓`, `Enter` | Adjust sliders, lists, color picker, buttons |
| `W` `A` `S` `d` | Orbit the model |
| `Space` / `M` / `R` | Auto-orbit / shading mode / reset view |
| `T` | Test vibration (if the controller supports it) |
| `h` / `l` · `o` · `g` · `n` · `x` | Diagnostics: prev/next test · start test · guided check · next step · stop all |
| `D` / `B` / `U` | Simulate hot-plug / transport switch / permission error (mock & test devices) |
| `?` | Scrollable Help (arrows, PgUp/PgDn, Home/End; Esc or ? closes) |
| `Esc` | Close modal / return Home / clear selection |
| `PgUp` / `PgDn` | Scroll long forms and diagnostics on short terminals |
| `Q` | Quit |

---

## License

MIT License

### Click to copy diagnostics

Click a diagnostic value, result message, device detail, firmware reading or log
line to copy its exact underlying text. Clipping and wrapping only affect the
display. Values exclude field labels and borders; diagnostic messages retain
relevant measurements. `Tab` / `Shift+Tab` focuses copy targets, and `Enter` (or
`Space`) copies the focused target. Existing sliders and controller stage
interactions keep their controls.

Diagnostics, Device Information and Device Details expose **Copy All
Diagnostics**. Diagnostics exposes **Copy Current Test**; firmware exposes **Copy Test Results**.
`y` copies the selected diagnostic category (including drift/deadzone results),
or the firmware report on Device Details; `Y` copies the full diagnostic report.
With no category selected, `y` copies the summary. Reports use current application
state, include unsupported/unmeasured features and timestamps, and mark simulated
data. They contain no audio recordings or raw HID dumps. Unique identifiers are
excluded or redacted by default. The **Identifiers: Off/On** action explicitly
opts into including them; switching devices resets this choice. Selecting an
individual identifier copies that identifier directly.

Clipboard backends:

- Linux Wayland: `wl-copy` from `wl-clipboard`, preferred when `WAYLAND_DISPLAY`
  is set. In mixed sessions, a failed Wayland copy falls back to X11 tools.
- Linux X11: `xclip -selection clipboard -in`, then `xsel --clipboard --input`,
  when `DISPLAY` is set. Native command failures are reported, not shown as success.
- macOS: `pbcopy`.
- Windows: PowerShell `Set-Clipboard`, with UTF-8 standard input and error checking.
- SSH / terminal fallback: explicitly start with `TUISENSE_OSC52=1 ./bin/tuisense --mock` in a terminal that permits OSC 52 clipboard writes.
  The clipboard request is base64 encoded, with tmux passthrough when inside tmux.
  tmux must permit passthrough (`allow-passthrough on`), and the outer terminal
  must permit clipboard access. Terminal requests are limited to 64 KiB of UTF-8
  text and show “request sent; paste to verify”, since OSC 52 writes have no
  acknowledgement. SSH sessions skip remote desktop backends to target the local
  terminal clipboard. Plain stdout output is never treated as clipboard success.

If no backend works, the exact text is saved to a private (0600) temporary UTF-8
file. An accessible dialog shows the full path and setup instructions; `Enter`
or `Esc` closes it. Open the file with a text editor to copy/share its contents,
and delete it after use. Individual copies can include explicitly selected
identifiers, including in fallback files.

To verify locally, run `make build`, then `./bin/tuisense --mock`. Open Diagnostics,
click a result/value and paste into a text editor. Try the two report buttons,
`y` / `Y`, and `Tab` then `Enter`. In Advanced Device Details, click the value to
copy it or its label to show its explanation. Test privacy with a MAC/serial
reading before and after opting in. `go test ./...` includes clipboard backend,
report privacy, mouse/focus and existing controller interaction tests. Building
locally does not change the installed `tuisense` command.

### Diagnostics 2.0

The diagnostics inspector scrolls independently of the centered controller stage.
Use PgUp/PgDn or the mouse wheel for detailed results. New tools include separate
circularity analysis, radial/axial deadzone preview, incomplete-test states, mapping
retests, timestamped events, Markdown/JSON export, and explicit local history
snapshots with compatibility-checked comparisons. No diagnostics are uploaded.
See [Diagnostics 2.0 methods, privacy, and validation](docs/DIAGNOSTICS_2.md).

Normal startup registers only real hardware drivers. `--mock` starts one explicitly
marked demo DualSense and skips hardware discovery. `--test-devices` explicitly
adds the simulated DualSense and Xbox development devices; they retain distinct
source labels and are never counted as physical hardware. `--no-test-devices`
remains a compatibility override. A disconnected normal session shows a clean
empty state and waits for hotplug, without enabling a demo automatically.

The minimal dashboard and device identity policy are documented in
[UI audit and redesign](docs/UI_REDESIGN.md).

### Explainable final health reports

Diagnostics now opens a **Health Report** after the guided sequence and can also
produce an explicitly partial report. Each test has its own summary and detailed
copy actions. Reports distinguish protocol completion, normal observations,
possible issues, and uncertain evidence; they include gameplay impact and safe
follow-up steps without a hardware-health percentage. Export supports plain text,
Markdown, and JSON. Saved historical reports can be reviewed, copied, and exported
without replacing their original observations with current values.

See [Health report architecture, coverage, and privacy](docs/HEALTH_REPORTS.md).
