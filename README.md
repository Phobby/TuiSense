# TuiSense

**TuiSense** is a terminal-native, **capability-driven controller management platform** built in Go on the [Limoni](https://github.com/thebanri/limoni) TUI & 3D engine. Its centerpiece is an interactive 3D controller in the middle of the screen; everything around it — status, device details, settings panels, even which tabs exist — adapts to the selected controller's declared capabilities and to the chosen **EZ** or **Advanced** mode.

The **Sony DualSense** (`054c:0ce6`) and **DualSense Edge** (`054c:0df2`) are the first real backends (Linux hidraw, USB & Bluetooth). A simulated **Xbox test controller** ships alongside to demonstrate that the same UI drives very different hardware.

---

## Highlights

- **Universal device model** — controllers are described by a `Descriptor` (identity, regions, 3D visual, actions) and a `CapabilitySet` (Battery, Connection, Firmware Info, Haptics, Adaptive Triggers, Lighting, Sticks, Motion, Audio, Profiles, Diagnostics, Firmware Update, Remapping, Calibration — each Full / Limited / Unsupported with sub-features).
- **Controller registry** — hardware drivers, mock devices, and test devices are registered in one place (`internal/drivers/builtin.go`). See [docs/ADDING_A_CONTROLLER.md](docs/ADDING_A_CONTROLLER.md).
- **Centered 3D stage** — the controller is the focal point; click its regions (or the callout pills) to open the matching settings. Switch controllers with the chips under the model, the header pill, `C`, or `,` / `.`.
- **EZ Mode** — a calm *Quick Settings* home with a few plain-language choices (light color, vibration strength, trigger feel, profile).
- **Advanced Mode** — full adaptive-trigger editor with force curve, per-motor haptics, RGB picker, stick deadzones & drift scope, IMU, audio, capability matrix, firmware tools, logs.
- **Per-device memory** — each controller keeps its own config; the selected mode, last device, and active profile per family persist in `settings.json`.
- **Diagnostics & Device Health Center** — a *Diagnostics* tab with a health summary, guided check, and per-component tests: analog stick drift (raw-count statistics, deadzone-aware interpretation, repeat confirmation), buttons (live highlight on the 3D model, guided press-each), triggers (rest/full travel, optional bounded adaptive resistance), touchpad, motion sensors, vibration, controller microphone & speaker (ALSA, matched by USB ID — never your PC's devices), battery (no fabricated health data), and connection (report rate, dropped reports). JSON/text report export in Advanced mode.
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
│   ├── system/                   # hidraw/sysfs discovery, udev, XDG
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
# Start interactive dashboard (auto-detects connected DualSense, with mock fallback):
tuisense

# Launch directly with the interactive mock DualSense controller:
tuisense --mock

# Launch with real hardware detection only (disables mock fallback):
tuisense --no-mock

# Hide the Xbox test device from the selector:
tuisense --no-test-devices

# Display available commands, flags, and XDG directories:
tuisense --help

# Display the installed application version:
tuisense --version

# Print the required Linux udev rules:
tuisense --print-udev
```

---

## Controls & Navigation

### Mouse
- **Click a controller region or callout pill** — selects it and opens the settings for its capability (in EZ mode, Advanced-only areas show a hint instead).
- **Click the device chips under the model, the header pill, or the stage title** — switch controllers.
- **Click EZ / ADVANCED in the header** — switch mode.
- **Drag / scroll on the stage** — orbit / zoom the 3D model.

### Keyboard
| Key | Action |
| :--- | :--- |
| `1`–`9`, `0` | Jump to a panel (numbers follow the visible tabs) |
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
| `?` | Help |
| `Esc` | Close modal → back to Overview → quit |
| `Q` | Quit |

---

## License

MIT License
