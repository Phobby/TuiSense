# Documentation media

The GIFs in this folder are recorded from the real TuiSense interface. Nothing is
drawn by hand or generated: every frame is the exact grid of characters and
colours TuiSense rendered.

## How they are made

1. `tools/capture` (Go) starts TuiSense headlessly with the built-in **simulated**
   DualSense — and, for `controller-switching`, the `--test-devices` simulators —
   then plays a scripted sequence of key presses, clicks and drags and records
   every frame. The simulated device is labelled **MOCK** in every frame; these
   recordings demonstrate the software, not real hardware.
2. `tools/capture/render.py` paints each frame on a fixed cell grid (DejaVu Sans
   Mono, Braille cells drawn as dots, a fallback font for missing symbols) and
   builds an optimised, looping GIF with `ffmpeg`.

```sh
make media                                               # everything below, in one step

go run -ldflags "-X github.com/Phobby/TuiSense/internal/version.Version=$(cat VERSION)" \
    ./tools/capture -out /tmp/tuisense-frames            # all scenes (or -scene NAME)
python3 tools/capture/render.py /tmp/tuisense-frames docs/media [NAME ...]
```

Requirements: Go, Python 3 with Pillow, `ffmpeg`, and the DejaVu fonts. The
`firmware-check` scene performs TuiSense's real, read-only firmware metadata
request, so it needs network access; without it the GIF shows *Unable to verify*.

## Conventions

| | |
| --- | --- |
| Terminal size | 110 × 32 cells (`customization`: 144 × 32; `ez-vs-advanced`: two 80 × 26 sessions) |
| Rendering | 9 × 18 px cells, scaled so every GIF is at most 1000 px wide |
| Theme | TuiSense default dark theme, English UI |
| Frame rate | 12 fps, about 8–12 s per clip |
| Size | under 2 MB each (checked by `internal/doccheck`) |

## Files

| File | Scene | Shows |
| --- | --- | --- |
| `dashboard.gif` | `dashboard` | Home screen with auto-rotation and hovering controller parts |
| `controller-3d.gif` | `controller-3d` | Drag-rotation, reset, gallery, Xbox Elite rear paddles |
| `ez-vs-advanced.gif` | `ez-vs-advanced` | The same steps in EZ (left) and Advanced (right) mode |
| `customization.gif` | `customization` | Light color, trigger feel and vibration test in EZ Customize |
| `diagnostics.gif` | `diagnostics` | Stick, trigger and button tests |
| `controller-switching.gif` | `controller-switching` | Controller selector, simulated test devices, gallery |
| `health-report.gif` | `health-report` | Stick test → health report → copy |
| `firmware-check.gif` | `firmware-check` | Device Details firmware check and official update guidance |
| `promo.gif` | — | Showcase video at the top of the README, converted from the promotional video with `ffmpeg` (6 fps, 640 px, 32 colours) |

## Not yet recorded

- **Real-hardware footage** (for example the DualSense light bar physically
  changing color) cannot be captured from the terminal and is not included.
